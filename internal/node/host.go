package node

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/fedishare/fedishare/internal/activitypub"
	"github.com/fedishare/fedishare/internal/apperr"
	"github.com/fedishare/fedishare/internal/config"
	"github.com/fedishare/fedishare/internal/federation"
	"github.com/fedishare/fedishare/internal/files"
	"github.com/fedishare/fedishare/internal/httpserver"
	"github.com/fedishare/fedishare/internal/status"
)

// Host is one desktop process: shared gateway and loopback UI, plus
// one Node per local actor/profile.
type Host struct {
	home string
	log  *slog.Logger
	app  *config.App

	mu       sync.Mutex
	nodes    []*Node
	selected string
	http     *http.Server
	localURL string
	empty    *status.Service
	started  bool
}

func NewHost(home string, log *slog.Logger) (*Host, error) {
	if home == "" {
		return nil, apperr.New(apperr.KindInvalidConfig, "FediShare is missing its data directory.")
	}
	if log == nil {
		log = slog.Default()
	}
	if err := config.EnsureHome(home); err != nil {
		return nil, err
	}
	if err := config.MigrateLegacyHome(home); err != nil {
		return nil, apperr.Wrap(apperr.KindFilesystem, "FediShare could not upgrade its profile layout.", err)
	}
	app, err := config.LoadApp(home)
	if err != nil {
		return nil, apperr.Wrap(apperr.KindInvalidConfig, "FediShare could not read app settings.", err)
	}
	empty := status.New()
	_ = empty.SetState(status.StateOffline)
	_ = empty.Update(func(snap *status.Snapshot) error {
		snap.Message = "Setup required"
		return nil
	})
	return &Host{home: home, log: log, app: app, empty: empty}, nil
}

func (h *Host) Start(ctx context.Context) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.started {
		return fmt.Errorf("host already started")
	}
	ids, err := config.ListProfileIDs(h.home)
	if err != nil {
		return err
	}
	for _, id := range ids {
		if err := h.bootProfileLocked(ctx, id); err != nil {
			h.stopNodesLocked(ctx)
			return err
		}
	}
	if h.selected == "" && len(h.nodes) > 0 {
		h.selected = h.nodes[0].ProfileID()
		h.app.SelectedID = h.selected
		_ = h.app.Save(h.home)
	}
	if err := h.startHTTPLocked(); err != nil {
		h.stopNodesLocked(ctx)
		return err
	}
	h.started = true
	h.log.Info("host started", "profiles", len(h.nodes), "dashboard", h.localURL)
	return nil
}

func (h *Host) bootProfileLocked(ctx context.Context, id string) error {
	phome := config.ProfileHome(h.home, id)
	cfg, err := config.Load(phome)
	if err != nil {
		return err
	}
	config.ApplyAppDefaults(cfg, *h.app)
	n, err := New(Options{
		Home:      phome,
		Config:    cfg,
		Log:       h.log,
		SkipHTTP:  true,
		ProfileID: id,
	})
	if err != nil {
		return err
	}
	if err := n.Start(ctx); err != nil {
		return err
	}
	h.nodes = append(h.nodes, n)
	if h.app.SelectedID == id || h.selected == "" {
		h.selected = id
	}
	return nil
}

func (h *Host) startHTTPLocked() error {
	handler, err := httpserver.New(h, h.log)
	if err != nil {
		return apperr.Wrap(apperr.KindInvalidConfig, "FediShare could not start its dashboard.", err)
	}
	ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", h.app.LocalPort))
	if err != nil {
		return apperr.Wrap(apperr.KindFilesystem, "FediShare could not open its local dashboard port.", err)
	}
	h.localURL = "http://" + ln.Addr().String()
	for _, n := range h.nodes {
		n.SetDashboardURL(h.localURL)
	}
	srv := &http.Server{
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    16 << 10,
	}
	h.http = srv
	go func() {
		if err := srv.Serve(ln); err != nil && err != http.ErrServerClosed {
			h.log.Error("local dashboard stopped", "err", err)
		}
	}()
	return nil
}

func (h *Host) Shutdown(ctx context.Context) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	var first error
	if h.http != nil {
		if err := h.http.Shutdown(ctx); err != nil {
			first = err
		}
		h.http = nil
	}
	if err := h.stopNodesLocked(ctx); err != nil && first == nil {
		first = err
	}
	h.started = false
	return first
}

func (h *Host) stopNodesLocked(ctx context.Context) error {
	var first error
	for _, n := range h.nodes {
		if err := n.Shutdown(ctx); err != nil && first == nil {
			first = err
		}
	}
	h.nodes = nil
	return first
}

func (h *Host) current() *Node {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.currentLocked()
}

func (h *Host) currentLocked() *Node {
	for _, n := range h.nodes {
		if n.ProfileID() == h.selected {
			return n
		}
	}
	if len(h.nodes) > 0 {
		return h.nodes[0]
	}
	return nil
}

func (h *Host) Status() *status.Service {
	if n := h.current(); n != nil {
		return n.Status()
	}
	return h.empty
}

func (h *Host) Config() *config.Config {
	if n := h.current(); n != nil {
		return n.Config()
	}
	cfg := config.Default()
	config.ApplyAppDefaults(&cfg, *h.app)
	return &cfg
}

func (h *Host) Home() string {
	if n := h.current(); n != nil {
		return n.Home()
	}
	return h.home
}

func (h *Host) DashboardURL() string {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.localURL
}

func (h *Host) NodeID() string {
	if n := h.current(); n != nil {
		return n.NodeID()
	}
	return ""
}

func (h *Host) ApplySetup(ctx context.Context, in httpserver.SetupRequest) error {
	h.mu.Lock()
	cur := h.currentLocked()
	h.mu.Unlock()
	if cur != nil && !cur.Config().Configured() {
		return cur.ApplySetup(ctx, h.withAppGateway(in))
	}
	return h.CreateProfile(ctx, in)
}

func (h *Host) withAppGateway(in httpserver.SetupRequest) httpserver.SetupRequest {
	if strings.TrimSpace(in.GatewayURL) == "" {
		in.GatewayURL = h.app.GatewayURL
	}
	h.app.GatewayURL = strings.TrimSpace(in.GatewayURL)
	_ = h.app.Save(h.home)
	return in
}

func (h *Host) CreateProfile(ctx context.Context, in httpserver.SetupRequest) error {
	in = h.withAppGateway(in)
	username := strings.ToLower(strings.TrimSpace(in.Username))
	share := strings.TrimSpace(in.ShareDirectory)
	if username == "" || share == "" {
		return apperr.New(apperr.KindInvalidConfig, "Choose a username and a shared folder.")
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	for _, n := range h.nodes {
		cfg := n.Config()
		if cfg.Username == username {
			return apperr.New(apperr.KindInvalidConfig, "That username is already used by another local profile.")
		}
		if cfg.ShareDirectory != "" && samePath(cfg.ShareDirectory, share) {
			return apperr.New(apperr.KindInvalidConfig, "That folder is already used by another local profile.")
		}
	}
	if err := config.ValidateShareRoot(share, h.home); err != nil {
		return apperr.Wrap(apperr.KindInvalidConfig, err.Error(), err)
	}
	id, err := config.NewProfileID()
	if err != nil {
		return err
	}
	phome := config.ProfileHome(h.home, id)
	if err := config.EnsureHome(phome); err != nil {
		return err
	}
	cfg := config.Default()
	config.ApplyAppDefaults(&cfg, *h.app)
	if err := cfg.Save(phome); err != nil {
		return err
	}
	n, err := New(Options{Home: phome, Config: &cfg, Log: h.log, SkipHTTP: true, ProfileID: id})
	if err != nil {
		return err
	}
	if err := n.Start(ctx); err != nil {
		return err
	}
	n.SetDashboardURL(h.localURL)
	if err := n.ApplySetup(ctx, in); err != nil {
		_ = n.Shutdown(ctx)
		return err
	}
	h.nodes = append(h.nodes, n)
	h.selected = id
	h.app.SelectedID = id
	return h.app.Save(h.home)
}

func (h *Host) UpdateSettings(ctx context.Context, in httpserver.SettingsRequest) error {
	if in.GatewayURL != nil || in.LocalPort != nil || in.LogLevel != nil || in.StartAtLogin != nil {
		h.mu.Lock()
		if in.GatewayURL != nil {
			h.app.GatewayURL = strings.TrimSpace(*in.GatewayURL)
		}
		if in.LocalPort != nil {
			h.app.LocalPort = *in.LocalPort
		}
		if in.LogLevel != nil {
			h.app.LogLevel = *in.LogLevel
		}
		if in.StartAtLogin != nil {
			h.app.StartAtLogin = *in.StartAtLogin
		}
		h.app.Normalize()
		_ = h.app.Save(h.home)
		gw := h.app.GatewayURL
		nodes := append([]*Node(nil), h.nodes...)
		h.mu.Unlock()
		for _, n := range nodes {
			copy := gw
			_ = n.UpdateSettings(ctx, httpserver.SettingsRequest{GatewayURL: &copy})
		}
	}
	n := h.current()
	if n == nil {
		return nil
	}
	in.GatewayURL = nil
	in.LocalPort = nil
	in.LogLevel = nil
	in.StartAtLogin = nil
	return n.UpdateSettings(ctx, in)
}

func (h *Host) Pause(ctx context.Context) error {
	if n := h.current(); n != nil {
		return n.Pause(ctx)
	}
	return apperr.New(apperr.KindInvalidConfig, "Finish setup before pausing sharing.")
}

func (h *Host) Resume(ctx context.Context) error {
	if n := h.current(); n != nil {
		return n.Resume(ctx)
	}
	return nil
}

func (h *Host) Rescan(ctx context.Context) error {
	if n := h.current(); n != nil {
		return n.Rescan(ctx)
	}
	return apperr.New(apperr.KindInvalidConfig, "Choose a shared folder first.")
}

func (h *Host) FileList() ([]files.Entry, error) {
	if n := h.current(); n != nil {
		return n.FileList()
	}
	return nil, nil
}

func (h *Host) FileHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if n := h.current(); n != nil {
			n.FileHandler().ServeHTTP(w, r)
			return
		}
		http.NotFound(w, r)
	})
}

func (h *Host) ActivityList() ([]activitypub.OutboxItem, error) {
	if n := h.current(); n != nil {
		return n.ActivityList()
	}
	return nil, nil
}

func (h *Host) FollowerList() ([]federation.Follower, error) {
	if n := h.current(); n != nil {
		return n.FollowerList()
	}
	return nil, nil
}

func (h *Host) Block(ctx context.Context, target string) error {
	if n := h.current(); n != nil {
		return n.Block(ctx, target)
	}
	return nil
}

func (h *Host) Unblock(ctx context.Context, target string) error {
	if n := h.current(); n != nil {
		return n.Unblock(ctx, target)
	}
	return nil
}

func (h *Host) Profiles() []config.ProfileInfo {
	h.mu.Lock()
	defer h.mu.Unlock()
	out := make([]config.ProfileInfo, 0, len(h.nodes))
	for _, n := range h.nodes {
		cfg := n.Config()
		out = append(out, config.ProfileInfo{
			ID:             n.ProfileID(),
			Username:       cfg.Username,
			DisplayName:    cfg.DisplayName,
			ShareDirectory: cfg.ShareDirectory,
			Identity:       cfg.FediverseAddress(),
			Selected:       n.ProfileID() == h.selected,
			Configured:     cfg.Configured(),
		})
	}
	return out
}

func (h *Host) SelectProfile(id string) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	for _, n := range h.nodes {
		if n.ProfileID() == id {
			h.selected = id
			h.app.SelectedID = id
			return h.app.Save(h.home)
		}
	}
	return apperr.New(apperr.KindInvalidConfig, "Unknown profile.")
}

func (h *Host) ServePublic(w http.ResponseWriter, r *http.Request) {
	user := r.PathValue("username")
	if user == "" {
		user = usernameFromPublicPath(r)
	}
	h.mu.Lock()
	var n *Node
	for _, cand := range h.nodes {
		if strings.EqualFold(cand.Config().Username, user) {
			n = cand
			break
		}
	}
	h.mu.Unlock()
	if n == nil {
		http.NotFound(w, r)
		return
	}
	n.PublicHandler().ServeHTTP(w, r)
}

func usernameFromPublicPath(r *http.Request) string {
	if r.URL.Path == "/.well-known/webfinger" {
		res := r.URL.Query().Get("resource")
		res = strings.TrimPrefix(strings.ToLower(res), "acct:")
		user, _, _ := strings.Cut(res, "@")
		return user
	}
	path := strings.TrimPrefix(r.URL.Path, "/users/")
	user, _, _ := strings.Cut(path, "/")
	return user
}

func samePath(a, b string) bool {
	return strings.EqualFold(strings.TrimRight(a, "/"), strings.TrimRight(b, "/"))
}

var _ httpserver.Backend = (*Host)(nil)
var _ httpserver.Profiles = (*Host)(nil)
var _ httpserver.PublicRouter = (*Host)(nil)

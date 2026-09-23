package node

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/fedishare/fedishare/internal/activitypub"
	"github.com/fedishare/fedishare/internal/activitystreams"
	"github.com/fedishare/fedishare/internal/apperr"
	"github.com/fedishare/fedishare/internal/config"
	"github.com/fedishare/fedishare/internal/crypto"
	"github.com/fedishare/fedishare/internal/database"
	"github.com/fedishare/fedishare/internal/federation"
	"github.com/fedishare/fedishare/internal/files"
	"github.com/fedishare/fedishare/internal/httpserver"
	"github.com/fedishare/fedishare/internal/identity"
	"github.com/fedishare/fedishare/internal/indexer"
	"github.com/fedishare/fedishare/internal/status"
)

const configKeyPaused = "sharing_paused"

// Options configure a desktop node.
type Options struct {
	Home   string
	Config *config.Config
	Log    *slog.Logger
	// AllowLocalFederation permits loopback/private fetches. Tests and
	// two nodes on one machine need this when a gateway URL is set.
	AllowLocalFederation bool
}

// Node owns local process lifecycle: config, SQLite, status, and the loopback UI.
type Node struct {
	home   string
	cfg    *config.Config
	log    *slog.Logger
	status *status.Service

	mu            sync.Mutex
	db            *database.DB
	http          *http.Server
	started       bool
	nodeID        string
	localURL      string
	paused        bool
	keys          crypto.KeyStore
	store         *files.Store
	indexer       *indexer.Indexer
	watch         *indexer.Watcher
	limiter       *files.Limiter
	fedStore      *federation.Store
	fedFetch      *federation.Fetcher
	inbox         *federation.Inbox
	publisher     *federation.Publisher
	worker        *federation.Worker
	runCtx        context.Context
	cancel        context.CancelFunc
	watchCtx      context.Context
	watchCancel   context.CancelFunc
	watchStarted  bool
	allowLocal    bool
	public        http.Handler
	gatewayUp     bool
	connecting    bool
	tunnelStarted bool
	tunnelCtx     context.Context
	tunnelCancel  context.CancelFunc
}

func New(opts Options) (*Node, error) {
	if opts.Home == "" {
		return nil, apperr.New(apperr.KindInvalidConfig, "FediShare is missing its data directory.")
	}
	cfg := opts.Config
	if cfg == nil {
		loaded, err := config.Load(opts.Home)
		if err != nil {
			return nil, apperr.Wrap(apperr.KindInvalidConfig, "FediShare could not read its configuration.", err)
		}
		cfg = loaded
	}
	log := opts.Log
	if log == nil {
		log = slog.Default()
	}
	return &Node{
		home:       opts.Home,
		cfg:        cfg,
		log:        log,
		status:     status.New(),
		keys:       crypto.KeyStore{Dir: config.KeysDir(opts.Home)},
		allowLocal: opts.AllowLocalFederation,
	}, nil
}

func (n *Node) Status() *status.Service { return n.status }

func (n *Node) Config() *config.Config { return n.cfg }

func (n *Node) Home() string { return n.home }

func (n *Node) DashboardURL() string {
	n.mu.Lock()
	defer n.mu.Unlock()
	return n.localURL
}

func (n *Node) NodeID() string {
	n.mu.Lock()
	defer n.mu.Unlock()
	return n.nodeID
}

func (n *Node) DB() *database.DB {
	n.mu.Lock()
	defer n.mu.Unlock()
	return n.db
}

func (n *Node) KeyStore() crypto.KeyStore { return n.keys }

func (n *Node) Paused() bool {
	n.mu.Lock()
	defer n.mu.Unlock()
	return n.paused
}

func (n *Node) Configured() bool { return n.cfg.Configured() }

func (n *Node) Username() string {
	n.mu.Lock()
	defer n.mu.Unlock()
	return n.cfg.Username
}

func (n *Node) DisplayName() string {
	n.mu.Lock()
	defer n.mu.Unlock()
	if n.cfg.DisplayName != "" {
		return n.cfg.DisplayName
	}
	return n.cfg.Username
}

func (n *Node) Summary() string {
	n.mu.Lock()
	defer n.mu.Unlock()
	return n.cfg.Summary
}

func (n *Node) PublicKeyPEM() (string, error) {
	return n.keys.PublicKeyPEM()
}

func (n *Node) PublicBase() string {
	n.mu.Lock()
	gw := n.cfg.GatewayURL
	local := n.localURL
	n.mu.Unlock()
	return config.PublicBase(gw, local)
}

func (n *Node) AcctHost() string {
	n.mu.Lock()
	gw := n.cfg.GatewayURL
	local := n.localURL
	n.mu.Unlock()
	return config.AcctHost(gw, local)
}

func (n *Node) ListPublicFiles(ctx context.Context, offset, limit int) ([]files.Record, int, error) {
	n.mu.Lock()
	store := n.store
	n.mu.Unlock()
	if store == nil {
		return nil, 0, nil
	}
	total, err := store.CountPublic(ctx)
	if err != nil {
		return nil, 0, err
	}
	recs, err := store.ListPublicPage(ctx, offset, limit)
	return recs, total, err
}

func (n *Node) GetPublicFile(ctx context.Context, id string) (files.Record, error) {
	n.mu.Lock()
	store := n.store
	n.mu.Unlock()
	if store == nil {
		return files.Record{}, files.ErrUnavailable
	}
	rec, err := store.GetByID(ctx, id)
	if err != nil {
		return files.Record{}, err
	}
	if !rec.Available || (rec.Visibility != "" && rec.Visibility != files.VisibilityPublic) {
		return files.Record{}, files.ErrUnavailable
	}
	return rec, nil
}

func (n *Node) ActivityList() ([]activitypub.OutboxItem, error) {
	if fed, err := n.federationActivityList(); err == nil && len(fed) > 0 {
		return fed, nil
	}
	recs, _, err := n.ListPublicFiles(context.Background(), 0, 50)
	if err != nil {
		return nil, err
	}
	paths := activitystreams.NewPaths(n.PublicBase(), n.Username())
	out := make([]activitypub.OutboxItem, 0, len(recs))
	for _, rec := range recs {
		published := rec.IndexedAt.UTC().Format("2006-01-02T15:04:05Z")
		if rec.IndexedAt.IsZero() {
			published = ""
		}
		out = append(out, activitypub.OutboxItem{
			ID:          paths.Activity(rec.ID),
			Type:        "Create",
			Name:        rec.Filename,
			Published:   published,
			ObjectURL:   "/users/" + paths.Username + "/files/" + rec.ID,
			DownloadURL: "/users/" + paths.Username + "/download/" + rec.ID,
		})
	}
	return out, nil
}

func (n *Node) FileHandler() http.Handler {
	return files.DownloadHandler(files.DownloadOptions{
		Root: func() string {
			return n.Config().ShareDirectory
		},
		Lookup: func(ctx context.Context, id string) (files.Record, error) {
			n.mu.Lock()
			store := n.store
			n.mu.Unlock()
			if store == nil {
				return files.Record{}, files.ErrUnavailable
			}
			return store.GetByID(ctx, id)
		},
		Paused:  n.Paused,
		Limiter: n.limiter,
	})
}

// Start initializes local storage, persists config.json if needed, and
// serves the loopback dashboard.
func (n *Node) Start(ctx context.Context) error {
	n.mu.Lock()
	defer n.mu.Unlock()
	if n.started {
		return fmt.Errorf("node already started")
	}
	if err := n.status.SetState(status.StateStarting); err != nil {
		return err
	}

	if err := config.EnsureHome(n.home); err != nil {
		return n.fail(apperr.Wrap(apperr.KindFilesystem, "FediShare could not create its data directory.", err))
	}

	n.cfg.Normalize()
	if err := n.cfg.Validate(); err != nil {
		return n.fail(apperr.Wrap(apperr.KindInvalidConfig, "FediShare settings are invalid.", err))
	}
	if !config.Exists(n.home) {
		if err := n.cfg.Save(n.home); err != nil {
			return n.fail(apperr.Wrap(apperr.KindFilesystem, "FediShare could not write its configuration.", err))
		}
		n.log.Info("wrote config.json on first run")
	}

	if err := n.keys.LoadOrCreate(); err != nil {
		return n.fail(apperr.Wrap(apperr.KindCrypto, "FediShare could not create its identity keys.", err))
	}

	db, err := database.Open(config.DatabasePath(n.home))
	if err != nil {
		return n.fail(apperr.Wrap(apperr.KindDatabase, "FediShare could not open its database.", err))
	}
	if err := db.Migrate(ctx); err != nil {
		_ = db.Close()
		return n.fail(apperr.Wrap(apperr.KindDatabase, "FediShare could not prepare its database.", err))
	}

	nodeID, err := identity.LoadOrCreateNodeID(ctx, db)
	if err != nil {
		_ = db.Close()
		return n.fail(apperr.Wrap(apperr.KindDatabase, "FediShare could not load its identity.", err))
	}
	if err := identity.RecordStart(ctx, db, time.Now().UTC().Format(time.RFC3339)); err != nil {
		_ = db.Close()
		return n.fail(apperr.Wrap(apperr.KindDatabase, "FediShare could not update its database.", err))
	}
	paused, err := loadPaused(ctx, db)
	if err != nil {
		_ = db.Close()
		return n.fail(apperr.Wrap(apperr.KindDatabase, "FediShare could not read its database.", err))
	}

	n.db = db
	n.nodeID = nodeID
	n.paused = paused
	n.store = files.NewStore(db)
	n.indexer = indexer.New(n.store, func() string { return n.Config().ShareDirectory }, n.status, n.log)
	n.watch = indexer.NewWatcher(n.indexer, n.log)
	n.limiter = files.NewLimiter(files.Limits{
		MaxConcurrent: n.cfg.MaxConcurrentDownloads,
		PerClient:     4,
		BandwidthBPS:  n.cfg.BandwidthLimitBPS,
	})
	n.runCtx, n.cancel = context.WithCancel(context.Background())
	n.startFederationLocked()

	if err := n.refreshLocked(false); err != nil {
		n.teardownLocked()
		return n.fail(err)
	}

	if err := n.startHTTPLocked(); err != nil {
		n.teardownLocked()
		return n.fail(err)
	}

	n.started = true
	if n.worker != nil && n.runCtx != nil {
		go n.worker.Run(n.runCtx)
	}
	if n.cfg.Configured() {
		n.ensureWatchLocked()
		n.ensureTunnelLocked()
	}
	n.log.Info("node started",
		"node_id", nodeID,
		"configured", n.cfg.Configured(),
		"state", n.status.Snapshot().State.String(),
		"dashboard", n.localURL,
	)
	return nil
}

func (n *Node) teardownLocked() {
	if n.watchCancel != nil {
		n.watchCancel()
		n.watchCancel = nil
	}
	if n.cancel != nil {
		n.cancel()
		n.cancel = nil
	}
	if n.db != nil {
		_ = n.db.Close()
		n.db = nil
	}
}

func (n *Node) ensureWatchLocked() {
	if n.watchStarted || n.indexer == nil {
		return
	}
	n.watchStarted = true
	n.watchCtx, n.watchCancel = context.WithCancel(context.Background())
	go n.runIndexLoop(n.watchCtx)
}

func (n *Node) runIndexLoop(ctx context.Context) {
	if err := n.status.SetState(status.StateIndexing); err != nil {
		n.log.Debug("index state", "err", err)
	}
	if err := n.indexer.Scan(ctx); err != nil && ctx.Err() == nil {
		n.log.Warn("initial file scan", "err", err)
	}
	n.indexer.PublishSummary(ctx)
	n.mu.Lock()
	_ = n.refreshLocked(false)
	n.mu.Unlock()
	if err := n.watch.Run(ctx); err != nil && ctx.Err() == nil {
		n.log.Warn("directory watch stopped", "err", err)
	}
}

func (n *Node) startHTTPLocked() error {
	handler, err := httpserver.New(n, n.log)
	if err != nil {
		return apperr.Wrap(apperr.KindInvalidConfig, "FediShare could not start its dashboard.", err)
	}
	ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", n.cfg.LocalPort))
	if err != nil {
		return apperr.Wrap(apperr.KindFilesystem, "FediShare could not open its local dashboard port.", err)
	}
	n.localURL = "http://" + ln.Addr().String()
	n.public = httpserver.PublicHandler(n)
	srv := &http.Server{
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    16 << 10,
		BaseContext: func(net.Listener) context.Context {
			return context.Background()
		},
	}
	n.http = srv
	go func() {
		if err := srv.Serve(ln); err != nil && err != http.ErrServerClosed {
			n.log.Error("local dashboard stopped", "err", err)
			_ = n.status.SetError("The local FediShare window could not stay open.")
		}
	}()
	return nil
}

func (n *Node) fail(err error) error {
	_ = n.status.SetError(apperr.UserMessage(err))
	n.log.Error("node failed to start", "err", err, "kind", apperr.Classify(err).String())
	return err
}

// Shutdown stops the dashboard and closes SQLite. It is idempotent.
func (n *Node) Shutdown(ctx context.Context) error {
	n.mu.Lock()
	defer n.mu.Unlock()

	_ = n.status.SetState(status.StateShuttingDown)
	_ = n.status.Update(func(snap *status.Snapshot) error {
		snap.Message = "Shutting down"
		snap.GatewayConnected = false
		snap.ActivityPubActive = false
		return nil
	})

	if n.watchCancel != nil {
		n.watchCancel()
		n.watchCancel = nil
	}
	if n.tunnelCancel != nil {
		n.tunnelCancel()
		n.tunnelCancel = nil
	}
	if n.cancel != nil {
		n.cancel()
		n.cancel = nil
	}
	if n.worker != nil {
		n.worker.Wait()
	}

	var first error
	if n.http != nil {
		if err := n.http.Shutdown(ctx); err != nil && first == nil {
			first = err
		}
		n.http = nil
	}
	if n.db != nil {
		if err := n.db.Close(); err != nil && first == nil {
			first = err
		}
		n.db = nil
	}
	n.started = false
	n.watchStarted = false
	n.tunnelStarted = false
	n.gatewayUp = false
	n.connecting = false
	n.log.Info("node stopped")
	return first
}

func (n *Node) ApplySetup(ctx context.Context, in httpserver.SetupRequest) error {
	n.mu.Lock()
	defer n.mu.Unlock()
	if n.cfg.Configured() {
		return apperr.New(apperr.KindInvalidConfig, "FediShare is already set up.")
	}
	next := *n.cfg
	next.DisplayName = in.DisplayName
	next.Username = in.Username
	next.ShareDirectory = in.ShareDirectory
	next.GatewayURL = in.GatewayURL
	next.Summary = in.Summary
	next.Normalize()
	if err := next.Validate(); err != nil {
		return apperr.Wrap(apperr.KindInvalidConfig, err.Error(), err)
	}
	if err := config.ValidateShareRoot(next.ShareDirectory, n.home); err != nil {
		return apperr.Wrap(apperr.KindInvalidConfig, err.Error(), err)
	}
	if next.Username == "" || next.ShareDirectory == "" {
		return apperr.New(apperr.KindInvalidConfig, "Choose a username and a shared folder.")
	}
	if err := os.MkdirAll(next.ShareDirectory, 0o755); err != nil {
		return apperr.Wrap(apperr.KindFilesystem, "FediShare could not create the shared folder.", err)
	}
	if err := next.Save(n.home); err != nil {
		return apperr.Wrap(apperr.KindFilesystem, "FediShare could not save its settings.", err)
	}
	n.cfg = &next
	if err := n.keys.LoadOrCreate(); err != nil {
		return apperr.Wrap(apperr.KindCrypto, "FediShare could not create its identity keys.", err)
	}
	n.ensureWatchLocked()
	n.ensureTunnelLocked()
	return n.refreshLocked(false)
}

func (n *Node) UpdateSettings(ctx context.Context, in httpserver.SettingsRequest) error {
	n.mu.Lock()
	defer n.mu.Unlock()
	next := *n.cfg
	oldShare := n.cfg.ShareDirectory
	oldGateway := n.cfg.GatewayURL
	if in.DisplayName != nil {
		next.DisplayName = *in.DisplayName
	}
	if in.Username != nil {
		next.Username = *in.Username
	}
	if in.Summary != nil {
		next.Summary = *in.Summary
	}
	if in.ShareDirectory != nil {
		next.ShareDirectory = *in.ShareDirectory
	}
	if in.GatewayURL != nil {
		next.GatewayURL = *in.GatewayURL
	}
	if in.LocalPort != nil {
		next.LocalPort = *in.LocalPort
	}
	if in.MaxConcurrentDownloads != nil {
		next.MaxConcurrentDownloads = *in.MaxConcurrentDownloads
	}
	if in.BandwidthLimitBPS != nil {
		next.BandwidthLimitBPS = *in.BandwidthLimitBPS
	}
	if in.LogLevel != nil {
		next.LogLevel = *in.LogLevel
	}
	if in.StartAtLogin != nil {
		next.StartAtLogin = *in.StartAtLogin
	}
	next.Normalize()
	if err := next.Validate(); err != nil {
		return apperr.Wrap(apperr.KindInvalidConfig, err.Error(), err)
	}
	if next.ShareDirectory != "" {
		if err := config.ValidateShareRoot(next.ShareDirectory, n.home); err != nil {
			return apperr.Wrap(apperr.KindInvalidConfig, err.Error(), err)
		}
		if err := os.MkdirAll(next.ShareDirectory, 0o755); err != nil {
			return apperr.Wrap(apperr.KindFilesystem, "FediShare could not create the shared folder.", err)
		}
	}
	if err := next.Save(n.home); err != nil {
		return apperr.Wrap(apperr.KindFilesystem, "FediShare could not save its settings.", err)
	}
	n.cfg = &next
	if n.limiter != nil {
		n.limiter.Limits.MaxConcurrent = next.MaxConcurrentDownloads
		n.limiter.Limits.BandwidthBPS = next.BandwidthLimitBPS
	}
	if next.ShareDirectory != oldShare && next.ShareDirectory != "" {
		if n.watchCancel != nil {
			n.watchCancel()
			n.watchCancel = nil
		}
		n.watchStarted = false
		n.ensureWatchLocked()
	}
	if next.GatewayURL != oldGateway {
		n.restartTunnelLocked()
	}
	return n.refreshLocked(false)
}

func (n *Node) Pause(_ context.Context) error {
	n.mu.Lock()
	defer n.mu.Unlock()
	if !n.cfg.Configured() {
		return apperr.New(apperr.KindInvalidConfig, "Finish setup before pausing sharing.")
	}
	n.paused = true
	if n.db != nil {
		if err := n.db.SetConfig(context.Background(), configKeyPaused, "1"); err != nil {
			return apperr.Wrap(apperr.KindDatabase, "FediShare could not save the pause state.", err)
		}
	}
	return n.refreshLocked(false)
}

func (n *Node) Resume(_ context.Context) error {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.paused = false
	if n.db != nil {
		if err := n.db.SetConfig(context.Background(), configKeyPaused, "0"); err != nil {
			return apperr.Wrap(apperr.KindDatabase, "FediShare could not save the pause state.", err)
		}
	}
	return n.refreshLocked(false)
}

func (n *Node) Rescan(ctx context.Context) error {
	n.mu.Lock()
	if !n.cfg.Configured() || n.indexer == nil {
		n.mu.Unlock()
		return apperr.New(apperr.KindInvalidConfig, "Choose a shared folder first.")
	}
	idx := n.indexer
	n.mu.Unlock()

	_ = n.status.SetState(status.StateIndexing)
	if err := idx.Scan(ctx); err != nil {
		return apperr.Wrap(apperr.KindFilesystem, "FediShare could not rescan the shared folder.", err)
	}
	idx.PublishSummary(ctx)
	n.mu.Lock()
	defer n.mu.Unlock()
	return n.refreshLocked(false)
}

func (n *Node) FileList() ([]files.Entry, error) {
	n.mu.Lock()
	store := n.store
	n.mu.Unlock()
	if store == nil || !n.cfg.Configured() {
		return nil, nil
	}
	recs, err := store.ListAvailable(context.Background(), 200)
	if err != nil {
		return nil, err
	}
	out := make([]files.Entry, 0, len(recs))
	for _, rec := range recs {
		out = append(out, rec.Entry())
	}
	return out, nil
}

func (n *Node) refreshLocked(_ bool) error {
	var sum files.Summary
	if n.store != nil && n.cfg.Configured() {
		if got, err := n.store.Summary(context.Background()); err == nil {
			sum = got
		}
	}

	next := status.StateOffline
	msg := "Setup required"
	if n.cfg.Configured() {
		msg = "Gateway not connected"
		n.gatewayMessage(&next, &msg)
	}
	if n.limiter != nil {
		_ = n.status.Update(func(snap *status.Snapshot) error {
			snap.ActiveDownloads = n.limiter.Active()
			return nil
		})
	}
	if n.fedStore != nil {
		if pending, err := n.fedStore.PendingCount(context.Background()); err == nil {
			_ = n.status.Update(func(snap *status.Snapshot) error {
				snap.PendingFederationJobs = pending
				return nil
			})
		}
	}
	if err := n.status.Update(func(snap *status.Snapshot) error {
		snap.NodeID = n.nodeID
		snap.Identity = n.cfg.FediverseAddress()
		snap.ShareDirectory = n.cfg.ShareDirectory
		snap.IndexedFiles = sum.Files
		snap.TotalBytes = sum.Bytes
		snap.ActivityPubActive = n.cfg.Configured()
		snap.GatewayConnected = n.gatewayUp
		snap.Message = msg
		return nil
	}); err != nil {
		return err
	}
	if err := n.status.SetState(next); err != nil {
		return err
	}
	return n.status.Update(func(snap *status.Snapshot) error {
		snap.Message = msg
		return nil
	})
}

func loadPaused(ctx context.Context, db *database.DB) (bool, error) {
	v, ok, err := db.GetConfig(ctx, configKeyPaused)
	if err != nil {
		return false, err
	}
	return ok && v == "1", nil
}

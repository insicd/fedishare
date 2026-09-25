package httpserver

import (
	"encoding/json"
	"errors"
	"html/template"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/fedishare/fedishare/internal/activitypub"
	"github.com/fedishare/fedishare/internal/activitystreams"
	"github.com/fedishare/fedishare/internal/apperr"
	"github.com/fedishare/fedishare/internal/config"
	"github.com/fedishare/fedishare/internal/desktop"
	"github.com/fedishare/fedishare/internal/federation"
	"github.com/fedishare/fedishare/internal/files"
	"github.com/fedishare/fedishare/internal/logging"
	"github.com/fedishare/fedishare/internal/status"
	"github.com/fedishare/fedishare/internal/version"
)

type pageData struct {
	Title          string
	CSRF           string
	Configured     bool
	Status         status.Snapshot
	Config         config.Config
	SuggestedShare string
	DashboardURL   string
	Version        string
	DefaultShare   string
	DefaultGateway string
	Profiles       []config.ProfileInfo
}

func (s *Server) page(tmpl *template.Template, _ string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cfg := *s.backend.Config()
		data := pageData{
			Title:          version.AppName,
			CSRF:           s.csrfToken,
			Configured:     cfg.Configured(),
			Status:         s.backend.Status().Snapshot(),
			Config:         cfg,
			SuggestedShare: config.DefaultShareDirectory(),
			DashboardURL:   s.backend.DashboardURL(),
			Version:        version.Version,
			DefaultShare:   config.DefaultShareDirectory(),
			DefaultGateway: config.DefaultGatewayURL,
			Profiles:       s.profiles(),
		}
		name := "dashboard.html"
		if !cfg.Configured() {
			name = "wizard.html"
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if err := tmpl.ExecuteTemplate(w, name, data); err != nil {
			s.log.Error("render page", "err", err)
			http.Error(w, "Could not render the FediShare window.", http.StatusInternalServerError)
		}
	}
}

func (s *Server) getNetwork(w http.ResponseWriter, r *http.Request) {
	n, ok := s.backend.(Network)
	if !ok {
		writeJSON(w, http.StatusOK, map[string]any{
			"type":   "FediShareNetwork",
			"actors": []any{},
			"hint":   "This desktop build cannot browse the FediShare network.",
		})
		return
	}
	dir, err := n.FetchNetwork(r.Context(), r.URL.Query().Get("q"))
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, dir)
}

func (s *Server) getStatus(w http.ResponseWriter, r *http.Request) {
	cfg := s.backend.Config()
	snap := s.backend.Status().Snapshot()
	writeJSON(w, http.StatusOK, map[string]any{
		"status":          snap,
		"configured":      cfg.Configured(),
		"identity":        cfg.FediverseAddress(),
		"share_directory": cfg.ShareDirectory,
		"gateway_url":     cfg.GatewayURL,
		"dashboard_url":   s.backend.DashboardURL(),
		"version":         version.Version,
		"profiles":        s.profiles(),
	})
}

func (s *Server) profiles() []config.ProfileInfo {
	p, ok := s.backend.(Profiles)
	if !ok {
		return nil
	}
	return p.Profiles()
}

func (s *Server) getProfiles(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"profiles": s.profiles()})
}

func (s *Server) postProfiles(w http.ResponseWriter, r *http.Request) {
	p, ok := s.backend.(Profiles)
	if !ok {
		http.Error(w, "profiles are not available", http.StatusNotImplemented)
		return
	}
	var in SetupRequest
	if err := decodeJSON(r, &in); err != nil {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}
	if err := p.CreateProfile(r.Context(), in); err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "profiles": s.profiles()})
}

func (s *Server) postSelectProfile(w http.ResponseWriter, r *http.Request) {
	p, ok := s.backend.(Profiles)
	if !ok {
		http.Error(w, "profiles are not available", http.StatusNotImplemented)
		return
	}
	var in struct {
		ID string `json:"id"`
	}
	if err := decodeJSON(r, &in); err != nil || strings.TrimSpace(in.ID) == "" {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}
	if err := p.SelectProfile(strings.TrimSpace(in.ID)); err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "profiles": s.profiles()})
}

func (s *Server) getFiles(w http.ResponseWriter, r *http.Request) {
	list, err := s.backend.FileList()
	if err != nil {
		writeError(w, err)
		return
	}
	if list == nil {
		list = []files.Entry{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"files": list})
}

func (s *Server) getFollowers(w http.ResponseWriter, r *http.Request) {
	list, err := s.backend.FollowerList()
	if err != nil {
		writeError(w, err)
		return
	}
	if list == nil {
		list = []federation.Follower{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": list})
}

type blockRequest struct {
	Target string `json:"target"`
}

func (s *Server) postBlock(w http.ResponseWriter, r *http.Request) {
	var in blockRequest
	if err := decodeJSON(r, &in); err != nil || strings.TrimSpace(in.Target) == "" {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}
	if err := s.backend.Block(r.Context(), strings.TrimSpace(in.Target)); err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) postUnblock(w http.ResponseWriter, r *http.Request) {
	var in blockRequest
	if err := decodeJSON(r, &in); err != nil || strings.TrimSpace(in.Target) == "" {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}
	if err := s.backend.Unblock(r.Context(), strings.TrimSpace(in.Target)); err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) getActivity(w http.ResponseWriter, r *http.Request) {
	list, err := s.backend.ActivityList()
	if err != nil {
		writeError(w, err)
		return
	}
	if list == nil {
		list = []activitypub.OutboxItem{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": list})
}

func (s *Server) getSettings(w http.ResponseWriter, r *http.Request) {
	cfg := s.backend.Config()
	writeJSON(w, http.StatusOK, cfg)
}

func (s *Server) getDiagnostics(w http.ResponseWriter, r *http.Request) {
	cfg := s.backend.Config()
	snap := s.backend.Status().Snapshot()
	writeJSON(w, http.StatusOK, map[string]any{
		"app":                 version.AppName,
		"version":             version.Version,
		"os":                  runtime.GOOS,
		"arch":                runtime.GOARCH,
		"go":                  runtime.Version(),
		"state":               snap.State.String(),
		"message":             snap.Message,
		"configured":          cfg.Configured(),
		"identity":            cfg.FediverseAddress(),
		"node_id":             s.backend.NodeID(),
		"share_directory":     logging.RedactPath(cfg.ShareDirectory),
		"data_directory":      logging.RedactPath(s.backend.Home()),
		"gateway_url":         cfg.GatewayURL,
		"gateway_connected":   snap.GatewayConnected,
		"activitypub_active":  snap.ActivityPubActive,
		"actor_url":           actorURL(cfg, s.backend.DashboardURL()),
		"webfinger":           webfingerHint(cfg, s.backend.DashboardURL()),
		"indexed_files":       snap.IndexedFiles,
		"total_bytes":         snap.TotalBytes,
		"pending_federation":  snap.PendingFederationJobs,
		"active_downloads":    snap.ActiveDownloads,
		"last_federation_err": snap.LastFederationError,
		"keys_present":        fileExists(filepath.Join(config.KeysDir(s.backend.Home()), "actor.pem")),
	})
}

func (s *Server) postBrowseFolder(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Start string `json:"start"`
	}
	if r.Body != nil {
		_ = json.NewDecoder(r.Body).Decode(&in)
	}
	path, err := desktop.ChooseFolder(in.Start)
	if errors.Is(err, desktop.ErrCanceled) {
		writeJSON(w, http.StatusOK, map[string]any{"cancelled": true})
		return
	}
	if err != nil {
		writeError(w, apperr.Wrap(apperr.KindFilesystem, err.Error(), err))
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"path": path})
}

func (s *Server) postSetup(w http.ResponseWriter, r *http.Request) {
	var in SetupRequest
	if err := decodeJSON(r, &in); err != nil {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}
	if err := s.backend.ApplySetup(r.Context(), in); err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":       true,
		"identity": s.backend.Config().FediverseAddress(),
	})
}

func (s *Server) putSettings(w http.ResponseWriter, r *http.Request) {
	var in SettingsRequest
	if err := decodeJSON(r, &in); err != nil {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}
	if err := s.backend.UpdateSettings(r.Context(), in); err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) postRescan(w http.ResponseWriter, r *http.Request) {
	if err := s.backend.Rescan(r.Context()); err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "status": s.backend.Status().Snapshot()})
}

func (s *Server) postPause(w http.ResponseWriter, r *http.Request) {
	if err := s.backend.Pause(r.Context()); err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "status": s.backend.Status().Snapshot()})
}

func (s *Server) postResume(w http.ResponseWriter, r *http.Request) {
	if err := s.backend.Resume(r.Context()); err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "status": s.backend.Status().Snapshot()})
}

func decodeJSON(r *http.Request, dest any) error {
	r.Body = http.MaxBytesReader(nil, r.Body, 1<<20)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	return dec.Decode(dest)
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, err error) {
	code := http.StatusBadRequest
	if apperr.Classify(err) == apperr.KindFilesystem || apperr.Classify(err) == apperr.KindDatabase {
		code = http.StatusInternalServerError
	}
	writeJSON(w, code, map[string]any{
		"error": apperr.UserMessage(err),
	})
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func actorURL(cfg *config.Config, fallback string) string {
	if cfg == nil || cfg.Username == "" {
		return ""
	}
	return activitystreams.NewPaths(cfg.PublicBase(fallback), cfg.Username).Actor()
}

func webfingerHint(cfg *config.Config, fallback string) string {
	if cfg == nil || cfg.Username == "" {
		return ""
	}
	host := cfg.AcctHost(fallback)
	if host == "" {
		return ""
	}
	return "acct:" + cfg.Username + "@" + host
}

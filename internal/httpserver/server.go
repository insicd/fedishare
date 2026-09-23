package httpserver

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"html/template"
	"io/fs"
	"log/slog"
	"net"
	"net/http"
	"strings"

	"github.com/fedishare/fedishare/internal/httpserver/web"
)

func New(backend Backend, log *slog.Logger) (http.Handler, error) {
	if log == nil {
		log = slog.Default()
	}
	token, err := randomToken(32)
	if err != nil {
		return nil, fmt.Errorf("csrf token: %w", err)
	}
	tmpl, err := parseTemplates()
	if err != nil {
		return nil, err
	}
	s := &Server{
		backend:   backend,
		log:       log,
		csrfToken: token,
	}
	mux := http.NewServeMux()
	static, err := fs.Sub(web.Static, "static")
	if err != nil {
		return nil, err
	}
	mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServer(http.FS(static))))
	mux.HandleFunc("GET /{$}", s.page(tmpl, "home"))
	mux.HandleFunc("GET /api/status", s.getStatus)
	mux.HandleFunc("GET /api/files", s.getFiles)
	mux.HandleFunc("GET /api/followers", s.getFollowers)
	mux.HandleFunc("POST /api/block", s.postBlock)
	mux.HandleFunc("POST /api/unblock", s.postUnblock)
	mux.HandleFunc("GET /api/activity", s.getActivity)
	mux.HandleFunc("GET /api/settings", s.getSettings)
	mux.HandleFunc("GET /api/diagnostics", s.getDiagnostics)
	mux.HandleFunc("POST /api/setup", s.postSetup)
	mux.HandleFunc("POST /api/rescan", s.postRescan)
	mux.HandleFunc("POST /api/pause", s.postPause)
	mux.HandleFunc("POST /api/resume", s.postResume)
	mux.HandleFunc("PUT /api/settings", s.putSettings)
	MountPublic(mux, backend)
	return s.wrap(mux), nil
}

func parseTemplates() (*template.Template, error) {
	return template.New("root").Funcs(template.FuncMap{
		"bytes": formatBytes,
	}).ParseFS(web.Templates, "templates/*.html")
}

func randomToken(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func (s *Server) wrap(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !localRequest(r) {
			http.Error(w, "FediShare's dashboard is only available on this computer.", http.StatusForbidden)
			return
		}
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "no-referrer")
		if !strings.HasPrefix(r.URL.Path, "/files/") &&
			!strings.HasPrefix(r.URL.Path, "/users/") &&
			!strings.HasPrefix(r.URL.Path, "/.well-known/") {
			w.Header().Set("Cache-Control", "no-store")
		}
		w.Header().Set("Content-Security-Policy", "default-src 'self'; style-src 'self'; script-src 'self'; img-src 'self' data:; connect-src 'self'")
		if r.Method != http.MethodGet && r.Method != http.MethodHead && r.Method != http.MethodOptions && !isInboxPOST(r) {
			if !allowedOrigin(r) {
				http.Error(w, "blocked cross-origin request", http.StatusForbidden)
				return
			}
			if !s.validCSRF(r) {
				http.Error(w, "missing or invalid CSRF token", http.StatusForbidden)
				return
			}
		}
		if !strings.HasPrefix(r.URL.Path, "/users/") &&
			!strings.HasPrefix(r.URL.Path, "/.well-known/") {
			http.SetCookie(w, &http.Cookie{
				Name:     "fedishare_csrf",
				Value:    s.csrfToken,
				Path:     "/",
				HttpOnly: false,
				SameSite: http.SameSiteStrictMode,
				Secure:   false,
			})
		}
		next.ServeHTTP(w, r)
	})
}

func localRequest(r *http.Request) bool {
	host := r.Host
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	}
	host = strings.Trim(host, "[]")
	switch strings.ToLower(host) {
	case "127.0.0.1", "localhost", "::1":
		return true
	default:
		return false
	}
}

func allowedOrigin(r *http.Request) bool {
	origin := strings.TrimSpace(r.Header.Get("Origin"))
	if origin == "" {
		return true
	}
	host := r.Host
	return origin == "http://"+host || origin == "https://"+host
}

func (s *Server) validCSRF(r *http.Request) bool {
	token := r.Header.Get("X-CSRF-Token")
	if token == "" {
		token = r.FormValue("csrf_token")
	}
	return token != "" && token == s.csrfToken
}

func isInboxPOST(r *http.Request) bool {
	if r.Method != http.MethodPost {
		return false
	}
	path := r.URL.Path
	return strings.HasPrefix(path, "/users/") && strings.HasSuffix(path, "/inbox")
}

func formatBytes(n int64) string {
	if n < 1024 {
		return fmt.Sprintf("%d B", n)
	}
	val := float64(n)
	units := []string{"KB", "MB", "GB", "TB"}
	for _, u := range units {
		val /= 1024
		if val < 1024 {
			return fmt.Sprintf("%.1f %s", val, u)
		}
	}
	return fmt.Sprintf("%.1f PB", val/1024)
}

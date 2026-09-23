package httpserver

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/fedishare/fedishare/internal/activitypub"
	"github.com/fedishare/fedishare/internal/config"
	"github.com/fedishare/fedishare/internal/federation"
	"github.com/fedishare/fedishare/internal/files"
	"github.com/fedishare/fedishare/internal/status"
)

type fakeBackend struct {
	cfg    *config.Config
	status *status.Service
	home   string
	setup  *SetupRequest
}

func (f *fakeBackend) Status() *status.Service          { return f.status }
func (f *fakeBackend) Config() *config.Config           { return f.cfg }
func (f *fakeBackend) Home() string                     { return f.home }
func (f *fakeBackend) DashboardURL() string             { return "http://127.0.0.1:17890" }
func (f *fakeBackend) NodeID() string                   { return "abc" }
func (f *fakeBackend) FileList() ([]files.Entry, error) { return nil, nil }
func (f *fakeBackend) ActivityList() ([]activitypub.OutboxItem, error) {
	return nil, nil
}
func (f *fakeBackend) FollowerList() ([]federation.Follower, error) { return nil, nil }
func (f *fakeBackend) Block(context.Context, string) error          { return nil }
func (f *fakeBackend) Unblock(context.Context, string) error        { return nil }
func (f *fakeBackend) FileHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	})
}
func (f *fakeBackend) Pause(context.Context) error  { return nil }
func (f *fakeBackend) Resume(context.Context) error { return nil }
func (f *fakeBackend) Rescan(context.Context) error { return nil }
func (f *fakeBackend) UpdateSettings(context.Context, SettingsRequest) error {
	return nil
}
func (f *fakeBackend) ApplySetup(_ context.Context, in SetupRequest) error {
	f.setup = &in
	f.cfg.Username = in.Username
	f.cfg.ShareDirectory = in.ShareDirectory
	f.cfg.DisplayName = in.DisplayName
	f.cfg.GatewayURL = in.GatewayURL
	return nil
}

func newTest(t *testing.T) (http.Handler, *fakeBackend) {
	t.Helper()
	cfg := config.Default()
	b := &fakeBackend{cfg: &cfg, status: status.New(), home: t.TempDir()}
	h, err := New(b, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	return h, b
}

func getCSRF(t *testing.T, h http.Handler) string {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "http://127.0.0.1:17890/api/status", nil)
	req.Host = "127.0.0.1:17890"
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatalf("status %d %s", rec.Code, rec.Body.Bytes())
	}
	for _, c := range rec.Result().Cookies() {
		if c.Name == "fedishare_csrf" {
			return c.Value
		}
	}
	t.Fatal("missing csrf cookie")
	return ""
}

func TestLocalHostAndCSRF(t *testing.T) {
	h, _ := newTest(t)
	cookie := getCSRF(t, h)

	badHost := httptest.NewRequest(http.MethodGet, "http://example.com/api/status", nil)
	badHost.Host = "example.com"
	badRec := httptest.NewRecorder()
	h.ServeHTTP(badRec, badHost)
	if badRec.Code != http.StatusForbidden {
		t.Fatalf("rebinding allowed: %d", badRec.Code)
	}

	post := httptest.NewRequest(http.MethodPost, "http://127.0.0.1:17890/api/setup", strings.NewReader(`{"username":"alice","share_directory":"/tmp/share","display_name":"Alice"}`))
	post.Host = "127.0.0.1:17890"
	post.Header.Set("Content-Type", "application/json")
	noCSRF := httptest.NewRecorder()
	h.ServeHTTP(noCSRF, post)
	if noCSRF.Code != http.StatusForbidden {
		t.Fatalf("csrf missing allowed: %d", noCSRF.Code)
	}

	post2 := httptest.NewRequest(http.MethodPost, "http://127.0.0.1:17890/api/setup", strings.NewReader(`{"username":"alice","share_directory":"/tmp/share","display_name":"Alice"}`))
	post2.Host = "127.0.0.1:17890"
	post2.Header.Set("Content-Type", "application/json")
	post2.Header.Set("X-CSRF-Token", cookie)
	ok := httptest.NewRecorder()
	h.ServeHTTP(ok, post2)
	if ok.Code != 200 {
		t.Fatalf("setup %d %s", ok.Code, ok.Body.Bytes())
	}

	cross := httptest.NewRequest(http.MethodPost, "http://127.0.0.1:17890/api/setup", strings.NewReader(`{}`))
	cross.Host = "127.0.0.1:17890"
	cross.Header.Set("Content-Type", "application/json")
	cross.Header.Set("Origin", "http://evil.example")
	cross.Header.Set("X-CSRF-Token", cookie)
	crossRec := httptest.NewRecorder()
	h.ServeHTTP(crossRec, cross)
	if crossRec.Code != http.StatusForbidden {
		t.Fatalf("origin allowed: %d", crossRec.Code)
	}
}

func TestWizardPageWhenUnconfigured(t *testing.T) {
	h, _ := newTest(t)
	req := httptest.NewRequest(http.MethodGet, "http://127.0.0.1:17890/", nil)
	req.Host = "127.0.0.1:17890"
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatal(rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "Set up FediShare") {
		t.Fatalf("expected wizard, got %s", rec.Body.String()[:min(200, rec.Body.Len())])
	}
}

func TestDiagnosticsOmitsSecrets(t *testing.T) {
	h, _ := newTest(t)
	req := httptest.NewRequest(http.MethodGet, "http://127.0.0.1:17890/api/diagnostics", nil)
	req.Host = "127.0.0.1:17890"
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	raw := rec.Body.String()
	for _, banned := range []string{"BEGIN PRIVATE", "private_key", "csrf"} {
		if strings.Contains(strings.ToLower(raw), strings.ToLower(banned)) {
			t.Fatalf("diagnostics leaked %q: %s", banned, raw)
		}
	}
	var payload map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
}

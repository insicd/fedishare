package node

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/fedishare/fedishare/internal/config"
	"github.com/fedishare/fedishare/internal/httpserver"
)

func TestHostTwoProfilesShareGateway(t *testing.T) {
	home := t.TempDir()
	app := config.DefaultApp()
	app.GatewayURL = ""
	app.LocalPort = 0
	if err := app.Save(home); err != nil {
		t.Fatal(err)
	}
	h, err := NewHost(home, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if err := h.Start(ctx); err != nil {
		t.Fatal(err)
	}
	defer h.Shutdown(ctx)

	aliceShare := filepath.Join(t.TempDir(), "alice")
	bobShare := filepath.Join(t.TempDir(), "bob")
	if err := h.CreateProfile(ctx, httpserver.SetupRequest{
		DisplayName: "Alice", Username: "alice", ShareDirectory: aliceShare,
	}); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(aliceShare, "a.txt"), []byte("a"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := h.CreateProfile(ctx, httpserver.SetupRequest{
		DisplayName: "Bob", Username: "bob", ShareDirectory: bobShare,
	}); err != nil {
		t.Fatal(err)
	}
	if err := h.CreateProfile(ctx, httpserver.SetupRequest{
		DisplayName: "Alice2", Username: "alice", ShareDirectory: filepath.Join(t.TempDir(), "x"),
	}); err == nil {
		t.Fatal("duplicate username")
	}
	list := h.Profiles()
	if len(list) != 2 {
		t.Fatalf("profiles=%+v", list)
	}
	if err := h.SelectProfile(list[0].ID); err != nil {
		t.Fatal(err)
	}
	if h.Config().Username != "alice" {
		t.Fatalf("selected=%q", h.Config().Username)
	}

	req := httptest.NewRequest(http.MethodGet, "http://127.0.0.1/users/bob", nil)
	req.SetPathValue("username", "bob")
	rec := httptest.NewRecorder()
	h.ServePublic(rec, req)
	if rec.Code != 200 || rec.Body.String() == "" {
		t.Fatalf("bob actor %d %s", rec.Code, rec.Body.String())
	}
}

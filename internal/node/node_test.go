package node

import (
	"context"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"github.com/fedishare/fedishare/internal/apperr"
	"github.com/fedishare/fedishare/internal/config"
	"github.com/fedishare/fedishare/internal/httpserver"
	"github.com/fedishare/fedishare/internal/identity"
	"github.com/fedishare/fedishare/internal/status"
)

func silentLog() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func ephemeral(t *testing.T) (*config.Config, string) {
	t.Helper()
	home := t.TempDir()
	cfg := config.Default()
	cfg.LocalPort = 0
	return &cfg, home
}

func TestStartCreatesConfigJSON(t *testing.T) {
	cfg, home := ephemeral(t)
	n, err := New(Options{Home: home, Config: cfg, Log: silentLog()})
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if err := n.Start(ctx); err != nil {
		t.Fatal(err)
	}
	defer n.Shutdown(ctx)

	if !config.Exists(home) {
		t.Fatal("first run did not create config.json")
	}
	if _, err := os.Stat(n.KeyStore().PrivatePath()); err != nil {
		t.Fatal(err)
	}
	loaded, err := config.Load(home)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Configured() {
		t.Fatal("defaults must stay unconfigured")
	}
	if loaded.LocalPort != 0 {
		t.Fatalf("port = %d", loaded.LocalPort)
	}
	if n.DashboardURL() == "" {
		t.Fatal("missing dashboard url")
	}
}

func TestStartUnconfiguredThenShutdown(t *testing.T) {
	cfg, home := ephemeral(t)
	n, err := New(Options{Home: home, Config: cfg, Log: silentLog()})
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if err := n.Start(ctx); err != nil {
		t.Fatal(err)
	}
	snap := n.Status().Snapshot()
	if snap.State != status.StateOffline {
		t.Fatalf("state = %s", snap.State)
	}
	if snap.Message != "Setup required" {
		t.Fatalf("message = %q", snap.Message)
	}
	if n.Config().Configured() {
		t.Fatal("should be unconfigured")
	}
	if err := n.Shutdown(ctx); err != nil {
		t.Fatal(err)
	}
	if n.Status().Snapshot().State != status.StateShuttingDown {
		t.Fatal("expected shutting down")
	}
}

func TestStartConfiguredPersistsNodeID(t *testing.T) {
	cfg, home := ephemeral(t)
	cfg.Username = "alice"
	cfg.ShareDirectory = filepath.Join(home, "share")
	cfg.GatewayURL = ""
	if err := cfg.Save(home); err != nil {
		t.Fatal(err)
	}

	n, err := New(Options{Home: home, Log: silentLog()})
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if err := n.Start(ctx); err != nil {
		t.Fatal(err)
	}
	id := n.NodeID()
	if n.Status().Snapshot().Message != "Gateway not connected" {
		t.Fatalf("message = %q", n.Status().Snapshot().Message)
	}
	if n.Status().Snapshot().Identity != "@alice" {
		t.Fatalf("identity = %q", n.Status().Snapshot().Identity)
	}
	if err := n.Shutdown(ctx); err != nil {
		t.Fatal(err)
	}

	n2, err := New(Options{Home: home, Log: silentLog()})
	if err != nil {
		t.Fatal(err)
	}
	if err := n2.Start(ctx); err != nil {
		t.Fatal(err)
	}
	defer n2.Shutdown(ctx)
	if n2.NodeID() != id {
		t.Fatalf("node id changed: %s vs %s", id, n2.NodeID())
	}
	got, ok, err := n2.DB().GetConfig(ctx, identity.ConfigKeyNodeID)
	if err != nil || !ok || got != id {
		t.Fatalf("db id %q ok=%v err=%v", got, ok, err)
	}
}

func TestApplySetupWritesConfigAndShareDir(t *testing.T) {
	cfg, home := ephemeral(t)
	n, err := New(Options{Home: home, Config: cfg, Log: silentLog()})
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if err := n.Start(ctx); err != nil {
		t.Fatal(err)
	}
	defer n.Shutdown(ctx)

	share := filepath.Join(t.TempDir(), "FediShare")
	err = n.ApplySetup(ctx, httpserver.SetupRequest{
		DisplayName:    "Alice",
		Username:       "Alice",
		ShareDirectory: share,
		GatewayURL:     "https://nodes.example.org",
		Summary:        "hello",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !n.Config().Configured() {
		t.Fatal("expected configured")
	}
	if n.Config().Username != "alice" {
		t.Fatalf("username %q", n.Config().Username)
	}
	loaded, err := config.Load(home)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.GatewayURL != "https://nodes.example.org" || loaded.ShareDirectory != share {
		t.Fatalf("saved %#v", loaded)
	}
	if _, err := os.Stat(share); err != nil {
		t.Fatal(err)
	}
	if n.Status().Snapshot().Identity != "@alice@nodes.example.org" {
		t.Fatalf("identity %q", n.Status().Snapshot().Identity)
	}
}

func TestPauseResumePersists(t *testing.T) {
	cfg, home := ephemeral(t)
	cfg.Username = "bob"
	cfg.ShareDirectory = filepath.Join(home, "share")
	cfg.GatewayURL = ""
	if err := os.Mkdir(cfg.ShareDirectory, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cfg.ShareDirectory, "a.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := cfg.Save(home); err != nil {
		t.Fatal(err)
	}
	n, err := New(Options{Home: home, Log: silentLog()})
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if err := n.Start(ctx); err != nil {
		t.Fatal(err)
	}
	if err := n.Rescan(ctx); err != nil {
		t.Fatal(err)
	}
	if n.Status().Snapshot().IndexedFiles != 1 {
		t.Fatalf("files = %d", n.Status().Snapshot().IndexedFiles)
	}
	if err := n.Pause(ctx); err != nil {
		t.Fatal(err)
	}
	if n.Status().Snapshot().State != status.StatePaused {
		t.Fatalf("state = %s", n.Status().Snapshot().State)
	}
	if err := n.Shutdown(ctx); err != nil {
		t.Fatal(err)
	}

	n2, err := New(Options{Home: home, Log: silentLog()})
	if err != nil {
		t.Fatal(err)
	}
	if err := n2.Start(ctx); err != nil {
		t.Fatal(err)
	}
	defer n2.Shutdown(ctx)
	if n2.Status().Snapshot().State != status.StatePaused {
		t.Fatalf("pause did not survive restart: %s", n2.Status().Snapshot().State)
	}
	if err := n2.Resume(ctx); err != nil {
		t.Fatal(err)
	}
	if n2.Status().Snapshot().State != status.StateOffline {
		t.Fatalf("state = %s", n2.Status().Snapshot().State)
	}
}

func TestStartRejectsInvalidConfig(t *testing.T) {
	home := t.TempDir()
	cfg := config.Default()
	cfg.LocalPort = 0
	cfg.LogLevel = "nope"
	n, err := New(Options{Home: home, Config: &cfg, Log: silentLog()})
	if err != nil {
		t.Fatal(err)
	}
	err = n.Start(context.Background())
	if err == nil {
		t.Fatal("expected invalid config")
	}
	if apperr.Classify(err) != apperr.KindInvalidConfig {
		t.Fatalf("kind = %s", apperr.Classify(err))
	}
	if n.Status().Snapshot().State != status.StateError {
		t.Fatalf("state = %s", n.Status().Snapshot().State)
	}
	if config.Exists(home) {
		t.Fatal("invalid first run should not write config.json")
	}
}

func TestDoubleStart(t *testing.T) {
	cfg, home := ephemeral(t)
	n, err := New(Options{Home: home, Config: cfg, Log: silentLog()})
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if err := n.Start(ctx); err != nil {
		t.Fatal(err)
	}
	defer n.Shutdown(ctx)
	if err := n.Start(ctx); err == nil {
		t.Fatal("expected already started")
	}
}

func TestNewRequiresHome(t *testing.T) {
	_, err := New(Options{})
	if err == nil {
		t.Fatal("expected error")
	}
}

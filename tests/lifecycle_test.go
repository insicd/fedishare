package tests

import (
	"context"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"github.com/fedishare/fedishare/internal/config"
	"github.com/fedishare/fedishare/internal/database"
	"github.com/fedishare/fedishare/internal/identity"
	"github.com/fedishare/fedishare/internal/node"
	"github.com/fedishare/fedishare/internal/status"
)

// TestPhase1Lifecycle is the end-to-end Phase 1 smoke test: home layout,
// config, SQLite, migrations, node id, status, and graceful shutdown.
func TestPhase1Lifecycle(t *testing.T) {
	home := t.TempDir()
	share := filepath.Join(home, "share")
	cfg := config.Default()
	cfg.Username = "alice"
	cfg.DisplayName = "Alice"
	cfg.LocalPort = 0
	cfg.ShareDirectory = share
	cfg.GatewayURL = ""
	if err := cfg.Save(home); err != nil {
		t.Fatal(err)
	}

	n, err := node.New(node.Options{
		Home: home,
		Log:  slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	if err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	if err := n.Start(ctx); err != nil {
		t.Fatal(err)
	}

	for _, path := range []string{
		config.Path(home),
		config.DatabasePath(home),
		config.KeysDir(home),
		filepath.Join(config.KeysDir(home), "actor.pem"),
	} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("%s: %v", path, err)
		}
	}

	snap := n.Status().Snapshot()
	if snap.State != status.StateOffline && snap.State != status.StateIndexing {
		t.Fatalf("state = %s", snap.State)
	}
	if snap.NodeID == "" || snap.Identity != "@alice" {
		t.Fatalf("snap = %+v", snap)
	}

	db := n.DB()
	if db == nil {
		t.Fatal("db not open")
	}
	id, ok, err := db.GetConfig(ctx, identity.ConfigKeyNodeID)
	if err != nil || !ok || id != snap.NodeID {
		t.Fatalf("persisted id %q ok=%v err=%v", id, ok, err)
	}

	// Re-open the same file after shutdown to prove WAL/migrations survive.
	if err := n.Shutdown(ctx); err != nil {
		t.Fatal(err)
	}
	reopened, err := database.Open(config.DatabasePath(home))
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	if err := reopened.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	got, ok, err := reopened.GetConfig(ctx, identity.ConfigKeyNodeID)
	if err != nil || !ok || got != id {
		t.Fatalf("reopened id %q ok=%v err=%v", got, ok, err)
	}
}

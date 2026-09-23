package identity

import (
	"context"
	"encoding/hex"
	"path/filepath"
	"testing"

	"github.com/fedishare/fedishare/internal/database"
)

func openDB(t *testing.T) *database.DB {
	t.Helper()
	db, err := database.Open(filepath.Join(t.TempDir(), "fedishare.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := db.Migrate(context.Background()); err != nil {
		t.Fatal(err)
	}
	return db
}

func TestLoadOrCreateNodeIDPersists(t *testing.T) {
	ctx := context.Background()
	db := openDB(t)

	first, err := LoadOrCreateNodeID(ctx, db)
	if err != nil {
		t.Fatal(err)
	}
	if len(first) != 32 {
		t.Fatalf("id length %d", len(first))
	}
	if _, err := hex.DecodeString(first); err != nil {
		t.Fatal(err)
	}
	second, err := LoadOrCreateNodeID(ctx, db)
	if err != nil {
		t.Fatal(err)
	}
	if first != second {
		t.Fatalf("id changed from %s to %s", first, second)
	}
}

func TestRecordStart(t *testing.T) {
	ctx := context.Background()
	db := openDB(t)
	if err := RecordStart(ctx, db, "2026-01-01T00:00:00Z"); err != nil {
		t.Fatal(err)
	}
	if err := RecordStart(ctx, db, "2026-01-02T00:00:00Z"); err != nil {
		t.Fatal(err)
	}
	first, ok, err := db.GetConfig(ctx, ConfigKeyFirstStarted)
	if err != nil || !ok || first != "2026-01-01T00:00:00Z" {
		t.Fatalf("first=%q ok=%v err=%v", first, ok, err)
	}
	last, ok, err := db.GetConfig(ctx, ConfigKeyLastStarted)
	if err != nil || !ok || last != "2026-01-02T00:00:00Z" {
		t.Fatalf("last=%q ok=%v err=%v", last, ok, err)
	}
}

package database

import (
	"context"
	"path/filepath"
	"testing"
)

func TestMigrateIdempotentAndConfigKV(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "fedishare.db")
	db, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	if err := db.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	if err := db.Migrate(ctx); err != nil {
		t.Fatal(err)
	}

	if _, ok, err := db.GetConfig(ctx, "missing"); err != nil || ok {
		t.Fatalf("missing key ok=%v err=%v", ok, err)
	}
	if err := db.SetConfig(ctx, "node_id", "abc"); err != nil {
		t.Fatal(err)
	}
	got, ok, err := db.GetConfig(ctx, "node_id")
	if err != nil || !ok || got != "abc" {
		t.Fatalf("got %q ok=%v err=%v", got, ok, err)
	}
	if err := db.SetConfig(ctx, "node_id", "def"); err != nil {
		t.Fatal(err)
	}
	got, ok, err = db.GetConfig(ctx, "node_id")
	if err != nil || !ok || got != "def" {
		t.Fatalf("update got %q ok=%v err=%v", got, ok, err)
	}
}

func TestReopenUsesExistingMigrations(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "fedishare.db")

	db, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	if err := db.SetConfig(ctx, "k", "v"); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	db, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := db.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	got, ok, err := db.GetConfig(ctx, "k")
	if err != nil || !ok || got != "v" {
		t.Fatalf("got %q ok=%v err=%v", got, ok, err)
	}
}

func TestParseVersion(t *testing.T) {
	v, err := parseVersion("001_initial.sql")
	if err != nil || v != 1 {
		t.Fatalf("v=%d err=%v", v, err)
	}
	if _, err := parseVersion("initial.sql"); err == nil {
		t.Fatal("expected error")
	}
}

func TestPing(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := db.Migrate(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := db.Ping(context.Background()); err != nil {
		t.Fatal(err)
	}
}

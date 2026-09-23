package indexer

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/fedishare/fedishare/internal/database"
	"github.com/fedishare/fedishare/internal/files"
)

func TestScanIndexesAndMarksDeleted(t *testing.T) {
	home := t.TempDir()
	share := t.TempDir()
	db, err := database.Open(filepath.Join(home, "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := db.Migrate(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(share, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(share, "sub", "a.txt"), []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(share, ".hidden"), []byte("no"), 0o644); err != nil {
		t.Fatal(err)
	}

	store := files.NewStore(db)
	idx := New(store, func() string { return share }, nil, nil)
	ctx := context.Background()
	if err := idx.Scan(ctx); err != nil {
		t.Fatal(err)
	}
	list, err := store.ListAvailable(ctx, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0].Filename != "a.txt" {
		t.Fatalf("list=%+v", list)
	}
	if list[0].Hash.Digest == "" || list[0].ID == list[0].RelativePath {
		t.Fatal("expected opaque id and hash")
	}

	if err := os.Remove(filepath.Join(share, "sub", "a.txt")); err != nil {
		t.Fatal(err)
	}
	if err := idx.Scan(ctx); err != nil {
		t.Fatal(err)
	}
	list, err = store.ListAvailable(ctx, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 0 {
		t.Fatalf("deleted file still available: %+v", list)
	}
}

func TestScanSkipsSymlink(t *testing.T) {
	home := t.TempDir()
	share := t.TempDir()
	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "secret.txt"), []byte("secret"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(outside, "secret.txt"), filepath.Join(share, "x.txt")); err != nil {
		t.Skip("symlinks not supported")
	}
	db, err := database.Open(filepath.Join(home, "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := db.Migrate(context.Background()); err != nil {
		t.Fatal(err)
	}
	idx := New(files.NewStore(db), func() string { return share }, nil, nil)
	if err := idx.Scan(context.Background()); err != nil {
		t.Fatal(err)
	}
	list, err := files.NewStore(db).ListAvailable(context.Background(), 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 0 {
		t.Fatalf("symlink indexed: %+v", list)
	}
}

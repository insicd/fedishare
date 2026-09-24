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

func TestScanMoveKeepsIDAndUpdatesPath(t *testing.T) {
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
	src := filepath.Join(share, "note.txt")
	if err := os.WriteFile(src, []byte("hello-move"), 0o644); err != nil {
		t.Fatal(err)
	}
	store := files.NewStore(db)
	idx := New(store, func() string { return share }, nil, nil)
	var kinds []string
	idx.SetHook(func(_ context.Context, ch Change) {
		kinds = append(kinds, ch.Kind)
	})
	ctx := context.Background()
	if err := idx.Scan(ctx); err != nil {
		t.Fatal(err)
	}
	first, err := store.ListAvailable(ctx, 10)
	if err != nil || len(first) != 1 {
		t.Fatalf("first=%v err=%v", first, err)
	}
	id := first[0].ID
	if err := os.Mkdir(filepath.Join(share, "docs"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(src, filepath.Join(share, "docs", "note.txt")); err != nil {
		t.Fatal(err)
	}
	kinds = nil
	if err := idx.Scan(ctx); err != nil {
		t.Fatal(err)
	}
	list, err := store.ListAvailable(ctx, 10)
	if err != nil || len(list) != 1 {
		t.Fatalf("after move list=%+v err=%v", list, err)
	}
	if list[0].ID != id {
		t.Fatalf("id changed: %s -> %s", id, list[0].ID)
	}
	if list[0].RelativePath != "docs/note.txt" {
		t.Fatalf("path=%q", list[0].RelativePath)
	}
	got, err := store.GetByID(ctx, id)
	if err != nil || !got.Available || got.RelativePath != "docs/note.txt" {
		t.Fatalf("get by id %+v err=%v", got, err)
	}
	for _, k := range kinds {
		if k == ChangeDelete {
			t.Fatalf("move emitted delete: %v", kinds)
		}
	}
	if len(kinds) != 1 || kinds[0] != ChangeUpdate {
		t.Fatalf("kinds=%v", kinds)
	}
}

func TestScanRecoversIDAfterBrokenMove(t *testing.T) {
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
	if err := os.Mkdir(filepath.Join(share, "docs"), 0o755); err != nil {
		t.Fatal(err)
	}
	abs := filepath.Join(share, "docs", "note.txt")
	if err := os.WriteFile(abs, []byte("hello-move"), 0o644); err != nil {
		t.Fatal(err)
	}
	sum, err := files.HashFile(abs)
	if err != nil {
		t.Fatal(err)
	}
	store := files.NewStore(db)
	ctx := context.Background()
	old, err := store.Upsert(ctx, files.Record{
		RelativePath: "note.txt",
		Filename:     "note.txt",
		MIMEType:     "text/plain",
		Size:         int64(len("hello-move")),
		Hash:         sum,
		Available:    false,
		Visibility:   files.VisibilityPublic,
	})
	if err != nil {
		t.Fatal(err)
	}
	idx := New(store, func() string { return share }, nil, nil)
	if err := idx.Scan(ctx); err != nil {
		t.Fatal(err)
	}
	got, err := store.GetByID(ctx, old.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !got.Available || got.RelativePath != "docs/note.txt" {
		t.Fatalf("not recovered: %+v", got)
	}
	list, err := store.ListAvailable(ctx, 10)
	if err != nil || len(list) != 1 || list[0].ID != old.ID {
		t.Fatalf("list=%+v err=%v", list, err)
	}
}

func TestScanCopyDoesNotStealID(t *testing.T) {
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
	if err := os.WriteFile(filepath.Join(share, "a.txt"), []byte("same"), 0o644); err != nil {
		t.Fatal(err)
	}
	store := files.NewStore(db)
	idx := New(store, func() string { return share }, nil, nil)
	ctx := context.Background()
	if err := idx.Scan(ctx); err != nil {
		t.Fatal(err)
	}
	first, _ := store.ListAvailable(ctx, 10)
	if err := os.WriteFile(filepath.Join(share, "b.txt"), []byte("same"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := idx.Scan(ctx); err != nil {
		t.Fatal(err)
	}
	list, err := store.ListAvailable(ctx, 10)
	if err != nil || len(list) != 2 {
		t.Fatalf("list=%+v err=%v", list, err)
	}
	if list[0].ID == list[1].ID {
		t.Fatal("duplicate copy reused id")
	}
	if first[0].ID != list[0].ID && first[0].ID != list[1].ID {
		t.Fatal("original id lost")
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

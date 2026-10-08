package files

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/fedishare/fedishare/internal/database"
)

func TestSearchPublic(t *testing.T) {
	db, err := database.Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ctx := context.Background()
	if err := db.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	store := NewStore(db)
	now := time.Now().UTC()
	for _, rec := range []Record{
		{RelativePath: "docs/manual.pdf", Filename: "manual.pdf", MIMEType: "application/pdf", Size: 10, Available: true, Visibility: VisibilityPublic, IndexedAt: now, Hash: ContentID{Algorithm: AlgoSHA256, Digest: "aa"}},
		{RelativePath: "photos/cat.jpg", Filename: "cat.jpg", MIMEType: "image/jpeg", Size: 20, Available: true, Visibility: VisibilityPublic, IndexedAt: now.Add(-time.Minute), Hash: ContentID{Algorithm: AlgoSHA256, Digest: "bb"}},
		{RelativePath: "secret.txt", Filename: "secret.txt", MIMEType: "text/plain", Size: 3, Available: true, Visibility: VisibilityPrivate, IndexedAt: now, Hash: ContentID{Algorithm: AlgoSHA256, Digest: "cc"}},
	} {
		if _, err := store.Upsert(ctx, rec); err != nil {
			t.Fatal(err)
		}
	}
	all, total, err := store.SearchPublic(ctx, "", 0, 10)
	if err != nil {
		t.Fatal(err)
	}
	if total != 2 || len(all) != 2 {
		t.Fatalf("all=%d total=%d", len(all), total)
	}
	hits, n, err := store.SearchPublic(ctx, "cat", 0, 10)
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 || len(hits) != 1 || hits[0].Filename != "cat.jpg" {
		t.Fatalf("search=%+v n=%d", hits, n)
	}
	docs, n, err := store.SearchPublic(ctx, "docs/", 0, 10)
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 || docs[0].Filename != "manual.pdf" {
		t.Fatalf("path search=%+v", docs)
	}
	page, n, err := store.SearchPublic(ctx, "", 1, 1)
	if err != nil || n != 2 || len(page) != 1 {
		t.Fatalf("page=%+v n=%d err=%v", page, n, err)
	}
}

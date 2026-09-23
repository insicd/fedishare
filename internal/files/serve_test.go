package files

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/fedishare/fedishare/internal/database"
)

func testStore(t *testing.T) (*Store, string) {
	t.Helper()
	home := t.TempDir()
	db, err := database.Open(filepath.Join(home, "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := db.Migrate(context.Background()); err != nil {
		t.Fatal(err)
	}
	return NewStore(db), home
}

func TestRangeAndHead(t *testing.T) {
	store, _ := testStore(t)
	root := t.TempDir()
	payload := []byte("hello world")
	if err := os.WriteFile(filepath.Join(root, "doc.txt"), payload, 0o644); err != nil {
		t.Fatal(err)
	}
	sum, err := HashFile(filepath.Join(root, "doc.txt"))
	if err != nil {
		t.Fatal(err)
	}
	rec, err := store.Upsert(context.Background(), Record{
		RelativePath: "doc.txt",
		Filename:     "doc.txt",
		MIMEType:     "text/plain; charset=utf-8",
		Size:         int64(len(payload)),
		Hash:         sum,
		Available:    true,
		Visibility:   VisibilityPublic,
	})
	if err != nil {
		t.Fatal(err)
	}

	h := DownloadHandler(DownloadOptions{
		Root:   func() string { return root },
		Lookup: store.GetByID,
		Paused: func() bool { return false },
	})
	mux := http.NewServeMux()
	mux.Handle("GET /files/{id}", h)
	mux.Handle("HEAD /files/{id}", h)

	head := httptest.NewRequest(http.MethodHead, "/files/"+rec.ID, nil)
	hr := httptest.NewRecorder()
	mux.ServeHTTP(hr, head)
	if hr.Code != 200 || hr.Header().Get("Accept-Ranges") != "bytes" {
		t.Fatalf("head %d %v", hr.Code, hr.Header())
	}
	if hr.Body.Len() != 0 {
		t.Fatal("head must not write a body")
	}

	req := httptest.NewRequest(http.MethodGet, "/files/"+rec.ID, nil)
	req.Header.Set("Range", "bytes=0-4")
	recw := httptest.NewRecorder()
	mux.ServeHTTP(recw, req)
	if recw.Code != http.StatusPartialContent {
		t.Fatalf("range status %d %s", recw.Code, recw.Body.String())
	}
	if recw.Body.String() != "hello" {
		t.Fatalf("body %q", recw.Body.String())
	}
	if recw.Header().Get("Content-Range") != "bytes 0-4/11" {
		t.Fatalf("content-range %s", recw.Header().Get("Content-Range"))
	}

	suf := httptest.NewRequest(http.MethodGet, "/files/"+rec.ID, nil)
	suf.Header.Set("Range", "bytes=-5")
	sr := httptest.NewRecorder()
	mux.ServeHTTP(sr, suf)
	if sr.Body.String() != "world" {
		t.Fatalf("suffix %q", sr.Body.String())
	}

	bad := httptest.NewRequest(http.MethodGet, "/files/"+rec.ID, nil)
	bad.Header.Set("Range", "bytes=99-120")
	br := httptest.NewRecorder()
	mux.ServeHTTP(br, bad)
	if br.Code != http.StatusRequestedRangeNotSatisfiable {
		t.Fatalf("bad range %d", br.Code)
	}
}

func TestServeRejectsTraversalIDAndPause(t *testing.T) {
	store, _ := testStore(t)
	root := t.TempDir()
	h := DownloadHandler(DownloadOptions{
		Root:   func() string { return root },
		Lookup: store.GetByID,
		Paused: func() bool { return true },
	})
	mux := http.NewServeMux()
	mux.Handle("GET /files/{id}", h)

	req := httptest.NewRequest(http.MethodGet, "/files/not-an-id", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != 404 {
		t.Fatalf("bad id %d", rec.Code)
	}

	rec2 := httptest.NewRecorder()
	mux.ServeHTTP(rec2, httptest.NewRequest(http.MethodGet, "/files/0123456789abcdef0123456789abcdef", nil))
	if rec2.Code != http.StatusServiceUnavailable {
		t.Fatalf("pause %d", rec2.Code)
	}
}

func TestServeMovedFile(t *testing.T) {
	store, _ := testStore(t)
	root := t.TempDir()
	path := filepath.Join(root, "gone.txt")
	if err := os.WriteFile(path, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	sum, _ := HashFile(path)
	rec, err := store.Upsert(context.Background(), Record{
		RelativePath: "gone.txt", Filename: "gone.txt", MIMEType: "text/plain",
		Size: 1, Hash: sum, Available: true, Visibility: VisibilityPublic,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	h := DownloadHandler(DownloadOptions{
		Root:   func() string { return root },
		Lookup: store.GetByID,
		Paused: func() bool { return false },
	})
	mux := http.NewServeMux()
	mux.Handle("GET /files/{id}", h)
	rw := httptest.NewRecorder()
	mux.ServeHTTP(rw, httptest.NewRequest(http.MethodGet, "/files/"+rec.ID, nil))
	if rw.Code != 404 {
		t.Fatalf("moved file %d", rw.Code)
	}
	_ = io.Discard
}

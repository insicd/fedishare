package tests

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/fedishare/fedishare/internal/config"
	"github.com/fedishare/fedishare/internal/httpserver"
	"github.com/fedishare/fedishare/internal/node"
)

func TestIndexedFileRangeDownload(t *testing.T) {
	home := t.TempDir()
	share := t.TempDir()
	if err := os.WriteFile(filepath.Join(share, "note.txt"), []byte("abcdefghij"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg := config.Default()
	cfg.LocalPort = 0
	n, err := node.New(node.Options{
		Home:   home,
		Config: &cfg,
		Log:    slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if err := n.Start(ctx); err != nil {
		t.Fatal(err)
	}
	defer n.Shutdown(ctx)

	if err := n.ApplySetup(ctx, httpserver.SetupRequest{
		DisplayName:    "Alice",
		Username:       "alice",
		ShareDirectory: share,
	}); err != nil {
		t.Fatal(err)
	}
	if err := n.Rescan(ctx); err != nil {
		t.Fatal(err)
	}
	list, err := n.FileList()
	if err != nil || len(list) != 1 {
		t.Fatalf("list=%v err=%v", list, err)
	}

	client := &http.Client{Timeout: 3 * time.Second}
	req, err := http.NewRequest(http.MethodGet, n.DashboardURL()+list[0].DownloadURL, nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Range", "bytes=2-5")
	res, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	body, _ := io.ReadAll(res.Body)
	if res.StatusCode != http.StatusPartialContent || string(body) != "cdef" {
		t.Fatalf("status=%d body=%q", res.StatusCode, body)
	}
}

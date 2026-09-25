package gateway

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/fedishare/fedishare/internal/network"
)

func TestNetworkDirectory(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "gateway.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	ctx := context.Background()
	if err := store.Register(ctx, "alice", "PEM-A", "n1"); err != nil {
		t.Fatal(err)
	}
	if err := store.Register(ctx, "bob", "PEM-B", "n2"); err != nil {
		t.Fatal(err)
	}
	if err := store.SaveActor(ctx, "alice", `{"name":"Alice","preferredUsername":"alice","summary":"hi","id":"https://nodes.example.org/users/alice"}`); err != nil {
		t.Fatal(err)
	}
	s := New(store, "https://nodes.example.org", slog.New(slog.NewTextHandler(io.Discard, nil)))
	req := httptest.NewRequest(http.MethodGet, "https://nodes.example.org/.well-known/fedishare-network", nil)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatalf("%d %s", rec.Code, rec.Body.String())
	}
	var dir network.Directory
	if err := json.Unmarshal(rec.Body.Bytes(), &dir); err != nil {
		t.Fatal(err)
	}
	if dir.Type != network.TypeDirectory || dir.Gateway != "https://nodes.example.org" {
		t.Fatalf("%+v", dir)
	}
	if len(dir.Actors) != 2 {
		t.Fatalf("actors=%+v", dir.Actors)
	}
	if dir.Actors[0].Username != "alice" || dir.Actors[0].Name != "Alice" || dir.Actors[0].Online {
		t.Fatalf("alice=%+v", dir.Actors[0])
	}
	if dir.Actors[1].Username != "bob" || dir.Actors[1].URL != "https://nodes.example.org/users/bob" {
		t.Fatalf("bob=%+v", dir.Actors[1])
	}
}

func TestStoreList(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "gateway.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	ctx := context.Background()
	if err := store.Register(ctx, "zoe", "P1", "n"); err != nil {
		t.Fatal(err)
	}
	if err := store.Register(ctx, "amy", "P2", "n"); err != nil {
		t.Fatal(err)
	}
	rows, err := store.List(ctx, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 || rows[0].Username != "amy" || rows[1].Username != "zoe" {
		t.Fatalf("%+v", rows)
	}
}

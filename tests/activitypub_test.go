package tests

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/fedishare/fedishare/internal/config"
	"github.com/fedishare/fedishare/internal/httpserver"
	"github.com/fedishare/fedishare/internal/node"
)

func TestLiveActorWebFingerOutbox(t *testing.T) {
	home := t.TempDir()
	share := t.TempDir()
	if err := os.WriteFile(filepath.Join(share, "note.txt"), []byte("hello"), 0o644); err != nil {
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
		GatewayURL:     "https://nodes.example.org",
		Summary:        "Sharing files.",
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
	base := n.DashboardURL()

	wf, err := client.Get(base + "/.well-known/webfinger?resource=acct:alice@nodes.example.org")
	if err != nil {
		t.Fatal(err)
	}
	defer wf.Body.Close()
	if wf.StatusCode != 200 {
		t.Fatalf("webfinger %s", wf.Status)
	}
	var jrd map[string]any
	if err := json.NewDecoder(wf.Body).Decode(&jrd); err != nil {
		t.Fatal(err)
	}
	if jrd["subject"] != "acct:alice@nodes.example.org" {
		t.Fatalf("subject=%v", jrd["subject"])
	}

	actor, err := client.Get(base + "/users/alice")
	if err != nil {
		t.Fatal(err)
	}
	defer actor.Body.Close()
	body, _ := io.ReadAll(actor.Body)
	if actor.StatusCode != 200 || !strings.Contains(string(body), `"preferredUsername":"alice"`) {
		t.Fatalf("actor %d %s", actor.StatusCode, body)
	}
	if !strings.Contains(string(body), "BEGIN PUBLIC KEY") {
		t.Fatalf("missing public key: %s", body)
	}

	outbox, err := client.Get(base + "/users/alice/outbox?page=1")
	if err != nil {
		t.Fatal(err)
	}
	defer outbox.Body.Close()
	outBody, _ := io.ReadAll(outbox.Body)
	if outbox.StatusCode != 200 || !strings.Contains(string(outBody), `"type":"Create"`) {
		t.Fatalf("outbox %d %s", outbox.StatusCode, outBody)
	}
	if !strings.Contains(string(outBody), "note.txt") {
		t.Fatalf("outbox missing file: %s", outBody)
	}

	note, err := client.Get(base + "/users/alice/notes/" + list[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	defer note.Body.Close()
	noteBody, _ := io.ReadAll(note.Body)
	if note.StatusCode != 200 || !strings.Contains(string(noteBody), `"type":"Note"`) || !strings.Contains(string(noteBody), "/followers") {
		t.Fatalf("note %d %s", note.StatusCode, noteBody)
	}

	doc, err := client.Get(base + "/users/alice/files/" + list[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	defer doc.Body.Close()
	docBody, _ := io.ReadAll(doc.Body)
	if doc.StatusCode != 200 || !strings.Contains(string(docBody), `"type":"Document"`) {
		t.Fatalf("document %d %s", doc.StatusCode, docBody)
	}

	dl, err := client.Get(base + "/users/alice/download/" + list[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	defer dl.Body.Close()
	bytes, _ := io.ReadAll(dl.Body)
	if dl.StatusCode != 200 || string(bytes) != "hello" {
		t.Fatalf("download %d %q", dl.StatusCode, bytes)
	}

	wrong, err := client.Get(base + "/users/bob")
	if err != nil {
		t.Fatal(err)
	}
	wrong.Body.Close()
	if wrong.StatusCode != 404 {
		t.Fatalf("wrong user %d", wrong.StatusCode)
	}

	if !n.Status().Snapshot().ActivityPubActive {
		t.Fatal("expected ActivityPub active after setup")
	}
}

package tests

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/fedishare/fedishare/internal/config"
	"github.com/fedishare/fedishare/internal/gateway"
	"github.com/fedishare/fedishare/internal/httpserver"
	"github.com/fedishare/fedishare/internal/node"
	"github.com/fedishare/fedishare/internal/status"
)

func TestGatewayTunnelActorDownloadAndOfflineCache(t *testing.T) {
	gwDir := t.TempDir()
	store, err := gateway.Open(filepath.Join(gwDir, "gateway.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	gw := gateway.New(store, "", slog.New(slog.NewTextHandler(io.Discard, nil)))
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	go func() { _ = gw.Serve(ln) }()
	gwURL := "http://" + ln.Addr().String()

	home := t.TempDir()
	share := t.TempDir()
	payload := strings.Repeat("fedishare-gateway-chunk-test\n", 4000) // ~112 KiB
	if err := os.WriteFile(filepath.Join(share, "note.txt"), []byte(payload), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg := config.Default()
	cfg.LocalPort = 0
	n, err := node.New(node.Options{
		Home:                 home,
		Config:               &cfg,
		Log:                  slog.New(slog.NewTextHandler(io.Discard, nil)),
		AllowLocalFederation: true,
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
		GatewayURL:     gwURL,
		Summary:        "Phase 6",
	}); err != nil {
		t.Fatal(err)
	}
	if err := n.Rescan(ctx); err != nil {
		t.Fatal(err)
	}
	files, err := n.FileList()
	if err != nil || len(files) != 1 {
		t.Fatalf("files=%v err=%v", files, err)
	}

	deadline := time.Now().Add(8 * time.Second)
	for {
		snap := n.Status().Snapshot()
		if snap.GatewayConnected && snap.State == status.StateOnline {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("tunnel did not come online: state=%s connected=%v msg=%q", snap.State, snap.GatewayConnected, snap.Message)
		}
		time.Sleep(50 * time.Millisecond)
	}

	client := &http.Client{Timeout: 5 * time.Second}
	wf, err := client.Get(gwURL + "/.well-known/webfinger?resource=acct:alice@" + ln.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer wf.Body.Close()
	if wf.StatusCode != 200 {
		body, _ := io.ReadAll(wf.Body)
		t.Fatalf("webfinger %s %s", wf.Status, body)
	}
	var jrd map[string]any
	if err := json.NewDecoder(wf.Body).Decode(&jrd); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(jrd["subject"].(string), "alice") {
		t.Fatalf("subject=%v", jrd["subject"])
	}

	actor, err := client.Get(gwURL + "/users/alice")
	if err != nil {
		t.Fatal(err)
	}
	defer actor.Body.Close()
	if actor.StatusCode != 200 {
		body, _ := io.ReadAll(actor.Body)
		t.Fatalf("actor %s %s", actor.Status, body)
	}
	var person map[string]any
	if err := json.NewDecoder(actor.Body).Decode(&person); err != nil {
		t.Fatal(err)
	}
	if person["preferredUsername"] != "alice" {
		t.Fatalf("actor=%v", person)
	}

	netRes, err := client.Get(gwURL + "/.well-known/fedishare-network")
	if err != nil {
		t.Fatal(err)
	}
	defer netRes.Body.Close()
	if netRes.StatusCode != 200 {
		body, _ := io.ReadAll(netRes.Body)
		t.Fatalf("network %s %s", netRes.Status, body)
	}
	var dir map[string]any
	if err := json.NewDecoder(netRes.Body).Decode(&dir); err != nil {
		t.Fatal(err)
	}
	actors, _ := dir["actors"].([]any)
	if dir["type"] != "FediShareNetwork" || len(actors) != 1 {
		t.Fatalf("directory=%v", dir)
	}
	alice, _ := actors[0].(map[string]any)
	if alice["username"] != "alice" || alice["online"] != true {
		t.Fatalf("alice=%v", alice)
	}

	htmlReq, err := http.NewRequest(http.MethodGet, gwURL+"/users/alice", nil)
	if err != nil {
		t.Fatal(err)
	}
	htmlReq.Header.Set("Accept", "text/html")
	htmlRes, err := client.Do(htmlReq)
	if err != nil {
		t.Fatal(err)
	}
	htmlBody, _ := io.ReadAll(htmlRes.Body)
	htmlRes.Body.Close()
	if htmlRes.StatusCode != 200 || !strings.Contains(htmlRes.Header.Get("Content-Type"), "text/html") {
		t.Fatalf("html actor %s %s", htmlRes.Status, htmlBody)
	}
	if !strings.Contains(string(htmlBody), "Alice") || !strings.Contains(string(htmlBody), "note.txt") {
		t.Fatalf("html profile=%s", htmlBody)
	}

	dl, err := client.Get(gwURL + "/users/alice/download/" + files[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	defer dl.Body.Close()
	if dl.StatusCode != 200 {
		body, _ := io.ReadAll(dl.Body)
		t.Fatalf("download %s %s", dl.Status, body)
	}
	got, err := io.ReadAll(dl.Body)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != payload {
		t.Fatalf("download length %d want %d", len(got), len(payload))
	}

	if err := n.Shutdown(ctx); err != nil {
		t.Fatal(err)
	}
	deadline = time.Now().Add(3 * time.Second)
	var offline *http.Response
	for {
		offline, err = client.Get(gwURL + "/users/alice/download/" + files[0].ID)
		if err != nil {
			t.Fatal(err)
		}
		if offline.StatusCode == http.StatusServiceUnavailable {
			break
		}
		offline.Body.Close()
		if time.Now().After(deadline) {
			t.Fatalf("expected 503 after node shutdown, got %s", offline.Status)
		}
		time.Sleep(50 * time.Millisecond)
	}
	offline.Body.Close()

	cached, err := client.Get(gwURL + "/users/alice")
	if err != nil {
		t.Fatal(err)
	}
	defer cached.Body.Close()
	if cached.StatusCode != 200 {
		body, _ := io.ReadAll(cached.Body)
		t.Fatalf("cached actor %s %s", cached.Status, body)
	}
	var cachedPerson map[string]any
	if err := json.NewDecoder(cached.Body).Decode(&cachedPerson); err != nil {
		t.Fatal(err)
	}
	if cachedPerson["preferredUsername"] != "alice" {
		t.Fatalf("html visit must not replace actor cache: %v", cachedPerson)
	}

	offHTML, err := http.NewRequest(http.MethodGet, gwURL+"/users/alice", nil)
	if err != nil {
		t.Fatal(err)
	}
	offHTML.Header.Set("Accept", "text/html")
	offRes, err := client.Do(offHTML)
	if err != nil {
		t.Fatal(err)
	}
	offBody, _ := io.ReadAll(offRes.Body)
	offRes.Body.Close()
	if offRes.StatusCode != 200 || !strings.Contains(offRes.Header.Get("Content-Type"), "text/html") {
		t.Fatalf("offline html %s ctype=%s %s", offRes.Status, offRes.Header.Get("Content-Type"), offBody)
	}
	if !strings.Contains(string(offBody), "Alice") || !strings.Contains(string(offBody), "not reachable") {
		t.Fatalf("offline html body %s", offBody)
	}
	cachedWF, err := client.Get(gwURL + "/.well-known/webfinger?resource=acct:alice@" + ln.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer cachedWF.Body.Close()
	if cachedWF.StatusCode != 200 {
		t.Fatalf("cached webfinger %s", cachedWF.Status)
	}

	if res, err := client.Get(gwURL + "/api/status"); err != nil {
		t.Fatal(err)
	} else {
		defer res.Body.Close()
		if res.StatusCode != http.StatusNotFound {
			t.Fatalf("admin leaked through gateway: %s", res.Status)
		}
	}
}

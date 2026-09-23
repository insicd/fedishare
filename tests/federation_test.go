package tests

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/fedishare/fedishare/internal/config"
	"github.com/fedishare/fedishare/internal/httpserver"
	"github.com/fedishare/fedishare/internal/httpsig"
	"github.com/fedishare/fedishare/internal/node"
)

func TestLiveFollowAcceptsAndListsFollower(t *testing.T) {
	home := t.TempDir()
	share := t.TempDir()
	if err := os.WriteFile(filepath.Join(share, "note.txt"), []byte("hi"), 0o644); err != nil {
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
		GatewayURL:     "https://nodes.example.org",
	}); err != nil {
		t.Fatal(err)
	}

	remoteKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	der, _ := x509.MarshalPKIXPublicKey(&remoteKey.PublicKey)
	pemStr := string(pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: der}))

	gotAccept := make(chan struct{}, 1)
	remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/users/bob" {
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":                "http://" + r.Host + "/users/bob",
				"type":              "Person",
				"preferredUsername": "bob",
				"inbox":             "http://" + r.Host + "/users/bob/inbox",
				"publicKey": map[string]any{
					"id":           "http://" + r.Host + "/users/bob#main-key",
					"publicKeyPem": pemStr,
				},
			})
			return
		}
		if r.URL.Path == "/users/bob/inbox" {
			select {
			case gotAccept <- struct{}{}:
			default:
			}
			w.WriteHeader(http.StatusAccepted)
			return
		}
		http.NotFound(w, r)
	}))
	defer remote.Close()
	bob := remote.URL + "/users/bob"

	follow := map[string]any{
		"@context": "https://www.w3.org/ns/activitystreams",
		"id":       bob + "/follows/1",
		"type":     "Follow",
		"actor":    bob,
		"object":   "https://nodes.example.org/users/alice",
	}
	body, _ := json.Marshal(follow)
	req, err := http.NewRequest(http.MethodPost, n.DashboardURL()+"/users/alice/inbox", strings.NewReader(string(body)))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/activity+json")
	if err := httpsig.SignRequest(req, bob+"#main-key", remoteKey, body); err != nil {
		t.Fatal(err)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != http.StatusAccepted {
		t.Fatalf("inbox %d", res.StatusCode)
	}

	list, err := n.FollowerList()
	if err != nil || len(list) != 1 || list[0].ActorID != bob {
		t.Fatalf("followers=%v err=%v", list, err)
	}

	select {
	case <-gotAccept:
	case <-time.After(3 * time.Second):
		t.Fatal("Accept was not delivered")
	}

	col, err := http.Get(n.DashboardURL() + "/users/alice/followers")
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := io.ReadAll(col.Body)
	col.Body.Close()
	if col.StatusCode != 200 || !strings.Contains(string(raw), `"totalItems":1`) {
		t.Fatalf("followers collection %d %s", col.StatusCode, raw)
	}
}

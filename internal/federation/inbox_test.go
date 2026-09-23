package federation

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"crypto/x509"
	"encoding/pem"

	"github.com/fedishare/fedishare/internal/activitystreams"
	"github.com/fedishare/fedishare/internal/database"
	"github.com/fedishare/fedishare/internal/httpsig"
)

func testDB(t *testing.T) *Store {
	t.Helper()
	db, err := database.Open(filepath.Join(t.TempDir(), "fedishare.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := db.Migrate(context.Background()); err != nil {
		t.Fatal(err)
	}
	return NewStore(db)
}

func TestFollowUndoAndReplay(t *testing.T) {
	store := testDB(t)
	remoteKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	pem := publicPEM(&remoteKey.PublicKey)

	remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/users/bob" {
			_ = json.NewEncoder(w).Encode(map[string]any{
				"@context":          "https://www.w3.org/ns/activitystreams",
				"id":                "http://" + r.Host + "/users/bob",
				"type":              "Person",
				"preferredUsername": "bob",
				"inbox":             "http://" + r.Host + "/users/bob/inbox",
				"publicKey": map[string]any{
					"id":           "http://" + r.Host + "/users/bob#main-key",
					"owner":        "http://" + r.Host + "/users/bob",
					"publicKeyPem": pem,
				},
			})
			return
		}
		http.NotFound(w, r)
	}))
	defer remote.Close()

	bob := remote.URL + "/users/bob"
	in := &Inbox{
		Store:   store,
		Fetcher: NewFetcher(true),
		Paths:   func() activitystreams.Paths { return activitystreams.NewPaths("https://nodes.example.org", "alice") },
		Enqueue: store.Enqueue,
	}

	follow := map[string]any{
		"@context": "https://www.w3.org/ns/activitystreams",
		"id":       bob + "/follows/1",
		"type":     "Follow",
		"actor":    bob,
		"object":   "https://nodes.example.org/users/alice",
	}
	body, _ := json.Marshal(follow)
	req := httptest.NewRequest(http.MethodPost, "http://127.0.0.1/users/alice/inbox", strings.NewReader(string(body)))
	req.Host = "127.0.0.1"
	if err := httpsig.SignRequest(req, bob+"#main-key", remoteKey, body); err != nil {
		t.Fatal(err)
	}
	if err := in.Process(context.Background(), req, body); err != nil {
		t.Fatal(err)
	}
	f, ok, err := store.GetFollower(context.Background(), bob)
	if err != nil || !ok || f.InboxURL == "" {
		t.Fatalf("follower stored ok=%v err=%v %+v", ok, err, f)
	}
	n, _ := store.PendingCount(context.Background())
	if n != 1 {
		t.Fatalf("expected Accept queued, pending=%d", n)
	}

	// Replay the same signature.
	if err := in.Process(context.Background(), req, body); err != ErrReplay {
		t.Fatalf("replay err=%v", err)
	}

	undo := map[string]any{
		"@context": "https://www.w3.org/ns/activitystreams",
		"id":       bob + "/undo/1",
		"type":     "Undo",
		"actor":    bob,
		"object": map[string]any{
			"id":     bob + "/follows/1",
			"type":   "Follow",
			"actor":  bob,
			"object": "https://nodes.example.org/users/alice",
		},
	}
	ubody, _ := json.Marshal(undo)
	ureq := httptest.NewRequest(http.MethodPost, "http://127.0.0.1/users/alice/inbox", strings.NewReader(string(ubody)))
	ureq.Host = "127.0.0.1"
	if err := httpsig.SignRequest(ureq, bob+"#main-key", remoteKey, ubody); err != nil {
		t.Fatal(err)
	}
	if err := in.Process(context.Background(), ureq, ubody); err != nil {
		t.Fatal(err)
	}
	if _, ok, _ := store.GetFollower(context.Background(), bob); ok {
		t.Fatal("follower should be gone")
	}
}

func TestUnsignedInboxRejected(t *testing.T) {
	store := testDB(t)
	in := &Inbox{
		Store:   store,
		Fetcher: NewFetcher(true),
		Paths:   func() activitystreams.Paths { return activitystreams.NewPaths("https://nodes.example.org", "alice") },
	}
	body := []byte(`{"type":"Follow","actor":"https://evil.example/users/x"}`)
	req := httptest.NewRequest(http.MethodPost, "http://127.0.0.1/users/alice/inbox", strings.NewReader(string(body)))
	req.Host = "127.0.0.1"
	if err := in.Process(context.Background(), req, body); err != ErrUnauthorized {
		t.Fatalf("err=%v", err)
	}
}

func publicPEM(pub *rsa.PublicKey) string {
	der, err := x509.MarshalPKIXPublicKey(pub)
	if err != nil {
		panic(err)
	}
	return string(pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: der}))
}

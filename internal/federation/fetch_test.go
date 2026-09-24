package federation

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestFetchActorSignsGETForAuthorizedFetch(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	var sawSig bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Signature") == "" {
			http.Error(w, "signature required", http.StatusUnauthorized)
			return
		}
		sawSig = true
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":                "http://" + r.Host + "/users/bob",
			"type":              "Person",
			"preferredUsername": "bob",
			"inbox":             "http://" + r.Host + "/users/bob/inbox",
			"publicKey": map[string]any{
				"id":           "http://" + r.Host + "/users/bob#main-key",
				"owner":        "http://" + r.Host + "/users/bob",
				"publicKeyPem": publicPEM(&key.PublicKey),
			},
		})
	}))
	defer srv.Close()

	unsigned := NewFetcher(true)
	if _, err := unsigned.FetchActor(context.Background(), srv.URL+"/users/bob"); err == nil || !strings.Contains(err.Error(), "401") {
		t.Fatalf("unsigned fetch err=%v", err)
	}

	signed := NewFetcher(true)
	signed.KeyID = func() string { return "https://nodes.example.org/users/alice#main-key" }
	signed.Private = func() (*rsa.PrivateKey, error) { return key, nil }
	actor, err := signed.FetchActor(context.Background(), srv.URL+"/users/bob")
	if err != nil {
		t.Fatal(err)
	}
	if !sawSig || actor.Username != "bob" {
		t.Fatalf("sawSig=%v actor=%+v", sawSig, actor)
	}
}

package httpsig

import (
	"crypto/rand"
	"crypto/rsa"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestSignAndVerifyRoundTrip(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	body := []byte(`{"type":"Follow"}`)
	req := httptest.NewRequest(http.MethodPost, "http://127.0.0.1:17890/users/alice/inbox", strings.NewReader(string(body)))
	req.Host = "127.0.0.1:17890"
	req.Header.Set("Host", "127.0.0.1:17890")
	if err := SignRequest(req, "https://remote.example/users/bob#main-key", key, body); err != nil {
		t.Fatal(err)
	}
	_, err = VerifyRequest(req, body, func(keyID string) (*rsa.PublicKey, error) {
		if keyID != "https://remote.example/users/bob#main-key" {
			t.Fatalf("keyId=%s", keyID)
		}
		return &key.PublicKey, nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestVerifyRejectsBadSignatureAndStaleDate(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	other, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	body := []byte(`{}`)
	req := httptest.NewRequest(http.MethodPost, "http://example/inbox", bytesReader(body))
	req.Host = "example"
	if err := SignRequest(req, "https://k#main-key", key, body); err != nil {
		t.Fatal(err)
	}
	_, err = VerifyRequest(req, body, func(string) (*rsa.PublicKey, error) { return &other.PublicKey, nil })
	if err == nil {
		t.Fatal("wrong key accepted")
	}

	req2 := httptest.NewRequest(http.MethodPost, "http://example/inbox", bytesReader(body))
	req2.Host = "example"
	if err := SignRequest(req2, "https://k#main-key", key, body); err != nil {
		t.Fatal(err)
	}
	req2.Header.Set("Date", time.Now().Add(-13*time.Hour).UTC().Format(http.TimeFormat))
	// resign would fix date; leave stale date with old signature over previous date — Verify checks Date first
	_, err = VerifyRequestAt(req2, body, func(string) (*rsa.PublicKey, error) { return &key.PublicKey, nil }, time.Now().UTC())
	if err == nil {
		t.Fatal("stale date accepted")
	}

	req3 := httptest.NewRequest(http.MethodPost, "http://example/inbox", bytesReader(body))
	req3.Host = "example"
	if err := SignRequest(req3, "https://k#main-key", key, body); err != nil {
		t.Fatal(err)
	}
	_, err = VerifyRequest(req3, []byte(`{"tampered":true}`), func(string) (*rsa.PublicKey, error) { return &key.PublicKey, nil })
	if err == nil {
		t.Fatal("digest mismatch accepted")
	}
}

func TestSignGETAndVerify(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodGet, "https://mastodon.social/users/alice", nil)
	req.Host = "mastodon.social"
	if err := SignGET(req, "https://nodes.example.org/users/bob#main-key", key); err != nil {
		t.Fatal(err)
	}
	if req.Header.Get("Digest") != "" {
		t.Fatal("GET signatures must not include Digest")
	}
	_, err = VerifyRequest(req, nil, func(string) (*rsa.PublicKey, error) { return &key.PublicKey, nil })
	if err != nil {
		t.Fatal(err)
	}
}

func TestParseRequestSkipsRFC9421Sibling(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "http://example/inbox", nil)
	req.Header.Add("Signature", `sig1=:YWJj:`)
	req.Header.Add("Signature", `keyId="https://remote.example/users/bob#main-key",algorithm="rsa-sha256",headers="date",signature="abc"`)
	p, err := ParseRequest(req)
	if err != nil {
		t.Fatal(err)
	}
	if p.KeyID != "https://remote.example/users/bob#main-key" {
		t.Fatalf("%+v", p)
	}
}

func bytesReader(b []byte) io.Reader { return strings.NewReader(string(b)) }

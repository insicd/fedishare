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

func bytesReader(b []byte) io.Reader { return strings.NewReader(string(b)) }

package tunnel

import (
	"crypto/rand"
	"crypto/rsa"
	"testing"
	"time"
)

func TestSignAndVerifyChallenge(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	ch, err := NewChallenge()
	if err != nil {
		t.Fatal(err)
	}
	sig, err := SignChallenge(key, "Alice", ch.Nonce, ch.IssuedAt)
	if err != nil {
		t.Fatal(err)
	}
	if err := VerifyChallenge(&key.PublicKey, "alice", ch.Nonce, ch.IssuedAt, sig); err != nil {
		t.Fatal(err)
	}
	if err := VerifyChallenge(&key.PublicKey, "bob", ch.Nonce, ch.IssuedAt, sig); err == nil {
		t.Fatal("expected username mismatch to fail")
	}
}

func TestCheckIssuedAt(t *testing.T) {
	now := time.Date(2026, 9, 23, 12, 0, 0, 0, time.UTC)
	if err := CheckIssuedAt(now.Format(time.RFC3339), now); err != nil {
		t.Fatal(err)
	}
	if err := CheckIssuedAt(now.Add(-3*time.Minute).Format(time.RFC3339), now); err == nil {
		t.Fatal("expected expired")
	}
	if err := CheckIssuedAt(now.Add(2*time.Minute).Format(time.RFC3339), now); err == nil {
		t.Fatal("expected future")
	}
}

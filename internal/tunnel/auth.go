package tunnel

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"strings"
	"time"
)

// ChallengeBytes is the exact payload the node signs.
func ChallengeBytes(username, nonce, issuedAt string) []byte {
	return []byte(ChallengeDomain + "\n" + strings.ToLower(strings.TrimSpace(username)) + "\n" + nonce + "\n" + issuedAt)
}

func NewChallenge() (Challenge, error) {
	var b [32]byte
	if _, err := rand.Read(b[:]); err != nil {
		return Challenge{}, err
	}
	return Challenge{
		Nonce:    hex.EncodeToString(b[:]),
		IssuedAt: time.Now().UTC().Format(time.RFC3339),
	}, nil
}

func SignChallenge(key *rsa.PrivateKey, username, nonce, issuedAt string) (string, error) {
	if key == nil {
		return "", fmt.Errorf("missing private key")
	}
	sum := sha256.Sum256(ChallengeBytes(username, nonce, issuedAt))
	sig, err := rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, sum[:])
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(sig), nil
}

func VerifyChallenge(pub *rsa.PublicKey, username, nonce, issuedAt, sigB64 string) error {
	if pub == nil {
		return fmt.Errorf("missing public key")
	}
	raw, err := base64.StdEncoding.DecodeString(sigB64)
	if err != nil {
		return fmt.Errorf("signature is not base64")
	}
	sum := sha256.Sum256(ChallengeBytes(username, nonce, issuedAt))
	if err := rsa.VerifyPKCS1v15(pub, crypto.SHA256, sum[:], raw); err != nil {
		return fmt.Errorf("challenge signature invalid")
	}
	return nil
}

func CheckIssuedAt(issuedAt string, now time.Time) error {
	t, err := time.Parse(time.RFC3339, issuedAt)
	if err != nil {
		return fmt.Errorf("invalid issued_at")
	}
	if t.After(now.Add(30 * time.Second)) {
		return fmt.Errorf("challenge is in the future")
	}
	if now.Sub(t) > 2*time.Minute {
		return fmt.Errorf("challenge expired")
	}
	return nil
}

package httpsig

import (
	"crypto"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"net/http"
	"strings"
	"time"
)

const (
	// MaxSkew is how far in the future a Date header may be.
	MaxSkew = 30 * time.Second
	// MaxAge is how old a Date header may be (replay window).
	MaxAge = 12 * time.Hour
)

// KeyFunc returns the RSA public key for a keyId.
type KeyFunc func(keyID string) (*rsa.PublicKey, error)

// VerifyRequest checks draft-cavage rsa-sha256, Date, and Digest.
func VerifyRequest(r *http.Request, body []byte, lookup KeyFunc) (Params, error) {
	return VerifyRequestAt(r, body, lookup, time.Now().UTC())
}

func VerifyRequestAt(r *http.Request, body []byte, lookup KeyFunc, now time.Time) (Params, error) {
	if lookup == nil {
		return Params{}, fmt.Errorf("missing key lookup")
	}
	p, err := Parse(r.Header.Get(HeaderName))
	if err != nil {
		return Params{}, err
	}
	if err := requireHeaders(p.Headers, r.Method); err != nil {
		return Params{}, err
	}
	if err := checkDate(r.Header.Get("Date"), now); err != nil {
		return Params{}, err
	}
	if contains(p.Headers, "digest") {
		got := r.Header.Get(DigestName)
		want := DigestSHA256(body)
		if !digestEqual(got, want) {
			return Params{}, fmt.Errorf("digest mismatch")
		}
	}
	signing, err := SigningString(r, p.Headers)
	if err != nil {
		return Params{}, err
	}
	raw, err := base64.StdEncoding.DecodeString(p.Signature)
	if err != nil {
		return Params{}, fmt.Errorf("signature is not base64")
	}
	pub, err := lookup(p.KeyID)
	if err != nil {
		return Params{}, err
	}
	if pub == nil {
		return Params{}, fmt.Errorf("unknown keyId")
	}
	sum := sha256.Sum256([]byte(signing))
	if err := rsa.VerifyPKCS1v15(pub, crypto.SHA256, sum[:], raw); err != nil {
		return Params{}, fmt.Errorf("signature verification failed")
	}
	return p, nil
}

func requireHeaders(headers []string, method string) error {
	need := []string{"date"}
	if strings.EqualFold(method, http.MethodPost) || strings.EqualFold(method, http.MethodPut) {
		need = append(need, "digest")
	}
	for _, h := range need {
		if !contains(headers, h) {
			return fmt.Errorf("signed headers must include %s", h)
		}
	}
	return nil
}

func checkDate(raw string, now time.Time) error {
	if raw == "" {
		return fmt.Errorf("missing Date header")
	}
	t, err := http.ParseTime(raw)
	if err != nil {
		return fmt.Errorf("invalid Date header")
	}
	if t.After(now.Add(MaxSkew)) {
		return fmt.Errorf("Date is in the future")
	}
	if now.Sub(t) > MaxAge {
		return fmt.Errorf("Date is too old")
	}
	return nil
}

func digestEqual(got, want string) bool {
	return strings.EqualFold(strings.TrimSpace(got), strings.TrimSpace(want))
}

func contains(list []string, want string) bool {
	for _, h := range list {
		if h == want {
			return true
		}
	}
	return false
}

// HashSignature is a stable id for replay protection (not the raw signature).
func HashSignature(sig string) string {
	sum := sha256.Sum256([]byte(sig))
	return fmt.Sprintf("%x", sum[:])
}

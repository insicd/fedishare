package httpsig

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"net/http"
	"strings"
	"time"
)

const (
	HeaderName = "Signature"
	DigestName = "Digest"
)

// DefaultHeaders are the headers Mastodon expects on signed POSTs.
var DefaultHeaders = []string{"(request-target)", "host", "date", "digest"}

// SignRequest adds Date, Digest, Host, and Signature on req.
// body must be the exact bytes that will be sent.
func SignRequest(req *http.Request, keyID string, key *rsa.PrivateKey, body []byte) error {
	if req == nil || key == nil || keyID == "" {
		return fmt.Errorf("missing signing material")
	}
	if req.Header.Get("Date") == "" {
		req.Header.Set("Date", time.Now().UTC().Format(http.TimeFormat))
	}
	if req.Host == "" && req.URL != nil {
		req.Host = req.URL.Host
	}
	if req.Header.Get("Host") == "" && req.Host != "" {
		req.Header.Set("Host", req.Host)
	}
	req.Header.Set(DigestName, DigestSHA256(body))
	signing, err := SigningString(req, DefaultHeaders)
	if err != nil {
		return err
	}
	sum := sha256.Sum256([]byte(signing))
	sig, err := rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, sum[:])
	if err != nil {
		return fmt.Errorf("sign: %w", err)
	}
	p := Params{
		KeyID:     keyID,
		Algorithm: "rsa-sha256",
		Headers:   DefaultHeaders,
		Signature: base64.StdEncoding.EncodeToString(sig),
	}
	req.Header.Set(HeaderName, p.Header())
	return nil
}

func DigestSHA256(body []byte) string {
	sum := sha256.Sum256(body)
	return "SHA-256=" + base64.StdEncoding.EncodeToString(sum[:])
}

func RequestTarget(r *http.Request) string {
	path := "/"
	if r.URL != nil {
		if r.URL.Opaque != "" {
			path = r.URL.Opaque
		} else if r.URL.Path != "" {
			path = r.URL.Path
		}
		if r.URL.RawQuery != "" {
			path += "?" + r.URL.RawQuery
		}
	}
	return strings.ToLower(r.Method) + " " + path
}

func SigningString(r *http.Request, headers []string) (string, error) {
	var lines []string
	for _, h := range headers {
		h = strings.ToLower(strings.TrimSpace(h))
		var val string
		switch h {
		case "(request-target)":
			val = RequestTarget(r)
		case "host":
			val = r.Host
			if val == "" {
				val = r.Header.Get("Host")
			}
		default:
			val = r.Header.Get(h)
		}
		if val == "" {
			return "", fmt.Errorf("signed header %q is missing", h)
		}
		lines = append(lines, h+": "+val)
	}
	return strings.Join(lines, "\n"), nil
}

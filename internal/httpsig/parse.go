// Package httpsig implements Mastodon-compatible HTTP Signatures
// (draft-cavage-http-signatures, rsa-sha256). RFC 9421 is not used.
package httpsig

import (
	"fmt"
	"strings"
)

// Params is a parsed Signature header.
type Params struct {
	KeyID     string
	Algorithm string
	Headers   []string
	Signature string
}

func Parse(header string) (Params, error) {
	header = strings.TrimSpace(header)
	if header == "" {
		return Params{}, fmt.Errorf("missing Signature header")
	}
	var p Params
	for _, part := range splitComma(header) {
		key, val, ok := strings.Cut(part, "=")
		if !ok {
			continue
		}
		key = strings.ToLower(strings.TrimSpace(key))
		val = strings.TrimSpace(val)
		val = strings.Trim(val, `"`)
		switch key {
		case "keyid":
			p.KeyID = val
		case "algorithm":
			p.Algorithm = strings.ToLower(val)
		case "headers":
			p.Headers = strings.Fields(strings.ToLower(val))
		case "signature":
			p.Signature = val
		}
	}
	if p.KeyID == "" || p.Signature == "" {
		return Params{}, fmt.Errorf("Signature header missing keyId or signature")
	}
	if len(p.Headers) == 0 {
		p.Headers = []string{"date"}
	}
	if p.Algorithm == "" {
		p.Algorithm = "rsa-sha256"
	}
	if p.Algorithm != "rsa-sha256" && p.Algorithm != "hs2019" {
		return Params{}, fmt.Errorf("unsupported signature algorithm %q", p.Algorithm)
	}
	return p, nil
}

func (p Params) Header() string {
	headers := strings.Join(p.Headers, " ")
	return fmt.Sprintf(`keyId="%s",algorithm="rsa-sha256",headers="%s",signature="%s"`,
		p.KeyID, headers, p.Signature)
}

func splitComma(s string) []string {
	var parts []string
	var b strings.Builder
	inQuote := false
	for _, r := range s {
		switch r {
		case '"':
			inQuote = !inQuote
			b.WriteRune(r)
		case ',':
			if inQuote {
				b.WriteRune(r)
				continue
			}
			parts = append(parts, b.String())
			b.Reset()
		default:
			b.WriteRune(r)
		}
	}
	if b.Len() > 0 {
		parts = append(parts, b.String())
	}
	return parts
}

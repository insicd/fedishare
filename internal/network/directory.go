package network

import (
	"html"
	"net/url"
	"strings"
)

const (
	// WellKnownPath is the public directory every FediShare gateway serves.
	WellKnownPath = "/.well-known/fedishare-network"
	// TypeDirectory is the JSON type of that document.
	TypeDirectory = "FediShareNetwork"
	maxSummary    = 280
	maxActors     = 500
)

// Directory is the public list of actors registered on one gateway.
type Directory struct {
	Type    string  `json:"type"`
	Gateway string  `json:"gateway"`
	Query   string  `json:"query,omitempty"`
	Hint    string  `json:"hint,omitempty"`
	Actors  []Actor `json:"actors"`
}

// Actor is one public FediShare (or looked-up) profile.
type Actor struct {
	Username string `json:"username"`
	Acct     string `json:"acct"`
	Name     string `json:"name,omitempty"`
	Summary  string `json:"summary,omitempty"`
	URL      string `json:"url"`
	Online   bool   `json:"online"`
}

// ActorFromDocument fills display fields from cached Actor JSON.
func ActorFromDocument(username, publicBase, raw string, online bool) Actor {
	username = strings.ToLower(strings.TrimSpace(username))
	base := strings.TrimRight(strings.TrimSpace(publicBase), "/")
	a := Actor{
		Username: username,
		Acct:     acctFor(username, base),
		Name:     username,
		URL:      base + "/users/" + username,
		Online:   online,
	}
	if raw == "" {
		return a
	}
	var doc struct {
		Name              string `json:"name"`
		PreferredUsername string `json:"preferredUsername"`
		Summary           string `json:"summary"`
		ID                string `json:"id"`
		URL               string `json:"url"`
	}
	if err := unmarshalActor(raw, &doc); err != nil {
		return a
	}
	if name := strings.TrimSpace(doc.Name); name != "" {
		a.Name = name
	} else if pref := strings.TrimSpace(doc.PreferredUsername); pref != "" {
		a.Name = pref
	}
	if sum := plainText(doc.Summary); sum != "" {
		a.Summary = sum
	}
	if u := strings.TrimSpace(doc.URL); u != "" {
		a.URL = u
	} else if id := strings.TrimSpace(doc.ID); id != "" {
		a.URL = id
	}
	return a
}

func acctFor(username, publicBase string) string {
	host := HostFromBase(publicBase)
	if username == "" || host == "" {
		return username
	}
	return username + "@" + host
}

// HostFromBase returns the host[:port] of a gateway or actor URL.
func HostFromBase(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	if !strings.Contains(raw, "://") {
		raw = "https://" + raw
	}
	u, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	return strings.ToLower(u.Host)
}

func plainText(s string) string {
	s = html.UnescapeString(s)
	var b strings.Builder
	in := false
	for _, r := range s {
		switch r {
		case '<':
			in = true
		case '>':
			in = false
		default:
			if !in {
				b.WriteRune(r)
			}
		}
	}
	out := strings.Join(strings.Fields(b.String()), " ")
	if len(out) > maxSummary {
		out = strings.TrimSpace(out[:maxSummary])
	}
	return out
}

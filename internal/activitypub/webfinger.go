package activitypub

import (
	"errors"
	"net"
	"net/url"
	"strings"
)

var (
	ErrBadResource     = errors.New("invalid WebFinger resource")
	ErrUnknownResource = errors.New("unknown WebFinger resource")
)

// JRD is a WebFinger JSON Resource Descriptor (RFC 7033).
type JRD struct {
	Subject string   `json:"subject"`
	Aliases []string `json:"aliases,omitempty"`
	Links   []Link   `json:"links"`
}

// Link is one WebFinger link.
type Link struct {
	Rel  string `json:"rel"`
	Type string `json:"type,omitempty"`
	Href string `json:"href,omitempty"`
}

type parsedResource struct {
	Username string
	Host     string
	ActorURL string
}

func parseResource(raw string) (parsedResource, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" || len(raw) > maxResourceLen {
		return parsedResource{}, ErrBadResource
	}
	if strings.ContainsAny(raw, " \t\r\n") {
		return parsedResource{}, ErrBadResource
	}

	lower := strings.ToLower(raw)
	if strings.HasPrefix(lower, "acct:") {
		rest := raw[len("acct:"):]
		user, host, ok := strings.Cut(rest, "@")
		if !ok || user == "" || host == "" {
			return parsedResource{}, ErrBadResource
		}
		host = normalizeHost(host)
		if host == "" || strings.Contains(user, "/") {
			return parsedResource{}, ErrBadResource
		}
		return parsedResource{
			Username: strings.ToLower(user),
			Host:     host,
		}, nil
	}

	if strings.HasPrefix(lower, "http://") || strings.HasPrefix(lower, "https://") {
		u, err := url.Parse(raw)
		if err != nil || u.Host == "" || u.Scheme != "http" && u.Scheme != "https" {
			return parsedResource{}, ErrBadResource
		}
		path := strings.TrimSuffix(u.Path, "/")
		const prefix = "/users/"
		if !strings.HasPrefix(path, prefix) {
			return parsedResource{}, ErrBadResource
		}
		user := strings.TrimPrefix(path, prefix)
		if user == "" || strings.Contains(user, "/") {
			return parsedResource{}, ErrBadResource
		}
		return parsedResource{
			Username: strings.ToLower(user),
			Host:     normalizeHost(u.Host),
			ActorURL: strings.TrimSuffix(u.Scheme+"://"+u.Host+path, "/"),
		}, nil
	}

	return parsedResource{}, ErrBadResource
}

func normalizeHost(host string) string {
	host = strings.TrimSpace(host)
	host = strings.Trim(host, "[]")
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	}
	return strings.ToLower(strings.Trim(host, "[]"))
}

func requestHost(rhost string) string {
	return normalizeHost(rhost)
}

func isLoopbackHost(host string) bool {
	switch strings.ToLower(host) {
	case "localhost", "127.0.0.1", "::1":
		return true
	default:
		return false
	}
}

func (p parsedResource) matches(username, acctHost, actorURL, reqHost string) bool {
	if !strings.EqualFold(p.Username, username) {
		return false
	}
	allowed := map[string]bool{
		strings.ToLower(acctHost): true,
	}
	if reqHost != "" {
		allowed[strings.ToLower(reqHost)] = true
	}
	if isLoopbackHost(acctHost) || isLoopbackHost(reqHost) {
		allowed["localhost"] = true
		allowed["127.0.0.1"] = true
		allowed["::1"] = true
	}
	if p.ActorURL != "" {
		if canonical(p.ActorURL) == canonical(actorURL) {
			return true
		}
		// Local dashboard fetches use the loopback origin, not the gateway.
		if isLoopbackHost(p.Host) && strings.EqualFold(p.Username, username) {
			return true
		}
		return false
	}
	return allowed[p.Host]
}

func canonical(raw string) string {
	return strings.TrimSuffix(strings.ToLower(strings.TrimSpace(raw)), "/")
}

func buildJRD(acct, actorURL string) JRD {
	return JRD{
		Subject: acct,
		Aliases: []string{actorURL},
		Links: []Link{
			{
				Rel:  "self",
				Type: "application/activity+json",
				Href: actorURL,
			},
			{
				Rel:  "http://webfinger.net/rel/profile-page",
				Type: "text/html",
				Href: actorURL,
			},
		},
	}
}

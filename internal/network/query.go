package network

import (
	"net/url"
	"regexp"
	"strings"
)

// Kind is how a lookup query should be resolved.
type Kind int

const (
	KindEmpty Kind = iota
	KindGateway
	KindAcct
	KindActor
	KindUnknown
)

var usernameRE = regexp.MustCompile(`^[A-Za-z0-9_]{1,30}$`)

// Classify decides whether q is a gateway URL, an actor URL, or an acct.
func Classify(q string) (Kind, string) {
	q = strings.TrimSpace(q)
	if q == "" {
		return KindEmpty, ""
	}
	q = strings.TrimPrefix(q, "@")
	lower := strings.ToLower(q)
	if strings.HasPrefix(lower, "acct:") {
		rest := strings.TrimSpace(q[len("acct:"):])
		if user, host, ok := splitAcct(rest); ok {
			return KindAcct, user + "@" + host
		}
		return KindUnknown, q
	}
	if strings.HasPrefix(lower, "http://") || strings.HasPrefix(lower, "https://") {
		u, err := url.Parse(q)
		if err != nil || u.Host == "" {
			return KindUnknown, q
		}
		path := strings.TrimSuffix(u.Path, "/")
		if path == "" || path == WellKnownPath {
			return KindGateway, u.Scheme + "://" + u.Host
		}
		if strings.HasPrefix(path, "/users/") {
			user := strings.TrimPrefix(path, "/users/")
			if user != "" && !strings.Contains(user, "/") {
				return KindActor, strings.TrimSuffix(u.Scheme+"://"+u.Host+path, "/")
			}
		}
		return KindGateway, u.Scheme + "://" + u.Host
	}
	if user, host, ok := splitAcct(q); ok {
		return KindAcct, user + "@" + host
	}
	if usernameRE.MatchString(q) {
		return KindAcct, strings.ToLower(q)
	}
	return KindUnknown, q
}

func splitAcct(raw string) (user, host string, ok bool) {
	raw = strings.TrimSpace(raw)
	user, host, found := strings.Cut(raw, "@")
	if !found || user == "" || host == "" {
		return "", "", false
	}
	if strings.Contains(user, "/") || strings.Contains(host, "/") {
		return "", "", false
	}
	user = strings.ToLower(user)
	host = strings.ToLower(strings.Trim(host, "[]"))
	if !usernameRE.MatchString(user) || host == "" {
		return "", "", false
	}
	return user, host, true
}

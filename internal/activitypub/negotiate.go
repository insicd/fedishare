package activitypub

import (
	"net/http"
	"strings"
)

// WantsHTML reports whether the caller is a browser (or similar) that
// prefers an HTML page over the ActivityPub JSON document.
//
// Federation clients send application/activity+json or ld+json and keep
// getting JSON. A missing Accept header also stays JSON so curl, tests,
// and older fetchers do not change behaviour.
func WantsHTML(r *http.Request) bool {
	if r == nil {
		return false
	}
	accept := strings.ToLower(r.Header.Get("Accept"))
	if strings.Contains(accept, "application/activity+json") || strings.Contains(accept, "application/ld+json") {
		return false
	}
	if strings.EqualFold(r.Header.Get("Sec-Fetch-Dest"), "document") {
		return true
	}
	if accept == "" {
		return false
	}
	return strings.Contains(accept, "text/html")
}

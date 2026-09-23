package httpserver

import "net/http"

import "github.com/fedishare/fedishare/internal/activitypub"

// MountPublic registers ActivityPub discovery and file-download routes.
// These are the only paths a gateway may reverse-proxy. The admin UI is
// never mounted here.
func MountPublic(mux *http.ServeMux, backend Backend) {
	filesH := backend.FileHandler()
	mux.Handle("GET /files/{id}", filesH)
	mux.Handle("HEAD /files/{id}", filesH)
	if src, ok := backend.(activitypub.Source); ok {
		activitypub.Mount(mux, src)
		gated := activitypub.ProtectUser(src, filesH)
		mux.Handle("GET /users/{username}/download/{id}", gated)
		mux.Handle("HEAD /users/{username}/download/{id}", gated)
	}
}

// PublicHandler is ActivityPub + downloads without CSRF or loopback Host
// checks. The gateway tunnel dispatches into this handler.
func PublicHandler(backend Backend) http.Handler {
	mux := http.NewServeMux()
	MountPublic(mux, backend)
	return mux
}

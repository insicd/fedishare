package activitypub

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/fedishare/fedishare/internal/activitystreams"
	"github.com/fedishare/fedishare/internal/federation"
	"github.com/fedishare/fedishare/internal/files"
)

// Handler serves WebFinger and ActivityPub GET endpoints for the local actor.
type Handler struct {
	src Source
}

func New(src Source) *Handler {
	return &Handler{src: src}
}

// Mount registers discovery and collection routes on mux.
func Mount(mux *http.ServeMux, src Source) {
	h := New(src)
	mux.HandleFunc("GET /.well-known/webfinger", h.WebFinger)
	mux.HandleFunc("GET /users/{username}", h.Actor)
	mux.HandleFunc("GET /users/{username}/outbox", h.Outbox)
	mux.HandleFunc("GET /users/{username}/followers", h.Followers)
	mux.HandleFunc("GET /users/{username}/following", h.Following)
	mux.HandleFunc("GET /users/{username}/inbox", h.Inbox)
	mux.HandleFunc("POST /users/{username}/inbox", h.InboxPOST)
	mux.HandleFunc("GET /users/{username}/files/{id}", h.FileObject)
	mux.HandleFunc("GET /users/{username}/notes/{id}", h.Note)
	mux.HandleFunc("GET /users/{username}/activities/{id}", h.Activity)
}

// ProtectUser runs next only when the path username is this node.
func ProtectUser(src Source, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !matchUser(src, r.PathValue("username")) {
			http.NotFound(w, r)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func matchUser(src Source, username string) bool {
	return src != nil && src.Configured() && src.Username() != "" &&
		strings.EqualFold(username, src.Username())
}

func (h *Handler) paths() activitystreams.Paths {
	return activitystreams.NewPaths(h.src.PublicBase(), h.src.Username())
}

func (h *Handler) WebFinger(w http.ResponseWriter, r *http.Request) {
	resource := r.URL.Query().Get("resource")
	parsed, err := parseResource(resource)
	if err != nil {
		http.Error(w, "invalid resource", http.StatusBadRequest)
		return
	}
	if !h.src.Configured() {
		http.NotFound(w, r)
		return
	}
	paths := h.paths()
	acctHost := h.src.AcctHost()
	if acctHost == "" {
		acctHost = requestHost(r.Host)
	}
	if !parsed.matches(h.src.Username(), acctHost, paths.Actor(), requestHost(r.Host)) {
		http.NotFound(w, r)
		return
	}
	writeJRD(w, http.StatusOK, buildJRD(paths.Acct(acctHost), paths.Actor()))
}

func (h *Handler) Actor(w http.ResponseWriter, r *http.Request) {
	if !matchUser(h.src, r.PathValue("username")) {
		http.NotFound(w, r)
		return
	}
	if WantsHTML(r) {
		h.writeProfile(w, r)
		return
	}
	pem, err := h.src.PublicKeyPEM()
	if err != nil {
		http.Error(w, "identity key is not available", http.StatusServiceUnavailable)
		return
	}
	actor := activitystreams.Person(h.paths(), h.src.DisplayName(), h.src.Summary(), pem)
	writeActivity(w, http.StatusOK, actor)
}

func (h *Handler) Outbox(w http.ResponseWriter, r *http.Request) {
	if !matchUser(h.src, r.PathValue("username")) {
		http.NotFound(w, r)
		return
	}
	paths := h.paths()
	page, ok, err := parsePage(r)
	if err != nil {
		http.Error(w, "invalid page", http.StatusBadRequest)
		return
	}
	if !ok {
		_, total, listErr := h.src.ListPublicFiles(r.Context(), 0, 1)
		if listErr != nil {
			http.Error(w, "could not read the outbox", http.StatusInternalServerError)
			return
		}
		writeActivity(w, http.StatusOK, activitystreams.Collection(paths.Outbox(), total, pageSize))
		return
	}

	offset := (page - 1) * pageSize
	recs, total, err := h.src.ListPublicFiles(r.Context(), offset, pageSize)
	if err != nil {
		http.Error(w, "could not read the outbox", http.StatusInternalServerError)
		return
	}
	items := make([]any, 0, len(recs))
	for _, rec := range recs {
		items = append(items, activitystreams.FileCreate(paths, rec))
	}
	pages := 0
	if total > 0 {
		pages = (total + pageSize - 1) / pageSize
	}
	var next, prev string
	if page < pages {
		next = paths.OutboxPage(page + 1)
	}
	if page > 1 {
		prev = paths.OutboxPage(page - 1)
	}
	writeActivity(w, http.StatusOK, activitystreams.CollectionPage(
		paths.OutboxPage(page), paths.Outbox(), next, prev, items,
	))
}

func (h *Handler) Followers(w http.ResponseWriter, r *http.Request) {
	h.iriCollection(w, r, h.paths().Followers(), func(offset, limit int) ([]string, int, error) {
		s, ok := h.src.(Social)
		if !ok {
			return nil, 0, nil
		}
		return s.ListFollowerIDs(r.Context(), offset, limit)
	})
}

func (h *Handler) Following(w http.ResponseWriter, r *http.Request) {
	h.iriCollection(w, r, h.paths().Following(), func(offset, limit int) ([]string, int, error) {
		s, ok := h.src.(Social)
		if !ok {
			return nil, 0, nil
		}
		return s.ListFollowingIDs(r.Context(), offset, limit)
	})
}

func (h *Handler) Inbox(w http.ResponseWriter, r *http.Request) {
	h.emptyCollection(w, r, h.paths().Inbox)
}

func (h *Handler) InboxPOST(w http.ResponseWriter, r *http.Request) {
	if !matchUser(h.src, r.PathValue("username")) {
		http.NotFound(w, r)
		return
	}
	social, ok := h.src.(Social)
	if !ok {
		http.Error(w, "inbox is not available", http.StatusNotImplemented)
		return
	}
	r.Body = http.MaxBytesReader(nil, r.Body, 1<<20)
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "request too large", http.StatusRequestEntityTooLarge)
		return
	}
	if err := social.ProcessInbox(r.Context(), r, body); err != nil {
		switch {
		case errors.Is(err, federation.ErrUnauthorized), errors.Is(err, federation.ErrReplay):
			http.Error(w, "signature required", http.StatusUnauthorized)
		case errors.Is(err, federation.ErrForbidden):
			http.Error(w, "forbidden", http.StatusForbidden)
		default:
			http.Error(w, "invalid activity", http.StatusBadRequest)
		}
		return
	}
	w.WriteHeader(http.StatusAccepted)
}

func (h *Handler) iriCollection(w http.ResponseWriter, r *http.Request, id string, listFn func(offset, limit int) ([]string, int, error)) {
	if !matchUser(h.src, r.PathValue("username")) {
		http.NotFound(w, r)
		return
	}
	page, wantPage, err := parsePage(r)
	if err != nil {
		http.Error(w, "invalid page", http.StatusBadRequest)
		return
	}
	if !wantPage {
		_, total, listErr := listFn(0, 1)
		if listErr != nil {
			http.Error(w, "could not read collection", http.StatusInternalServerError)
			return
		}
		writeActivity(w, http.StatusOK, activitystreams.Collection(id, total, pageSize))
		return
	}
	offset := (page - 1) * pageSize
	iris, total, err := listFn(offset, pageSize)
	if err != nil {
		http.Error(w, "could not read collection", http.StatusInternalServerError)
		return
	}
	items := make([]any, 0, len(iris))
	for _, iri := range iris {
		items = append(items, iri)
	}
	pages := 0
	if total > 0 {
		pages = (total + pageSize - 1) / pageSize
	}
	var next, prev string
	if page < pages {
		next = id + "?page=" + strconv.Itoa(page+1)
	}
	if page > 1 {
		prev = id + "?page=" + strconv.Itoa(page-1)
	}
	writeActivity(w, http.StatusOK, activitystreams.CollectionPage(
		id+"?page="+strconv.Itoa(page), id, next, prev, items,
	))
}

func (h *Handler) emptyCollection(w http.ResponseWriter, r *http.Request, idFn func() string) {
	if !matchUser(h.src, r.PathValue("username")) {
		http.NotFound(w, r)
		return
	}
	writeActivity(w, http.StatusOK, activitystreams.Collection(idFn(), 0, pageSize))
}

func (h *Handler) FileObject(w http.ResponseWriter, r *http.Request) {
	rec, ok := h.publicFile(w, r)
	if !ok {
		return
	}
	paths := h.paths()
	writeActivity(w, http.StatusOK, activitystreams.Document(
		paths.File(rec.ID), rec.Filename, rec.MIMEType, paths.Download(rec.ID), rec,
	))
}

func (h *Handler) Note(w http.ResponseWriter, r *http.Request) {
	rec, ok := h.publicFile(w, r)
	if !ok {
		return
	}
	if WantsHTML(r) {
		h.writeNotePage(w, rec)
		return
	}
	writeActivity(w, http.StatusOK, activitystreams.FileNote(h.paths(), rec))
}

func (h *Handler) Activity(w http.ResponseWriter, r *http.Request) {
	rec, ok := h.publicFile(w, r)
	if !ok {
		return
	}
	writeActivity(w, http.StatusOK, activitystreams.FileCreate(h.paths(), rec))
}

func (h *Handler) publicFile(w http.ResponseWriter, r *http.Request) (files.Record, bool) {
	if !matchUser(h.src, r.PathValue("username")) {
		http.NotFound(w, r)
		return files.Record{}, false
	}
	id := r.PathValue("id")
	if !files.ValidOpaqueID(id) {
		http.NotFound(w, r)
		return files.Record{}, false
	}
	rec, err := h.src.GetPublicFile(r.Context(), id)
	if err != nil {
		http.NotFound(w, r)
		return files.Record{}, false
	}
	return rec, true
}

func parsePage(r *http.Request) (page int, ok bool, err error) {
	raw := strings.TrimSpace(r.URL.Query().Get("page"))
	if raw == "" {
		return 0, false, nil
	}
	if raw == "true" {
		return 1, true, nil
	}
	n, convErr := strconv.Atoi(raw)
	if convErr != nil || n < 1 {
		if convErr == nil {
			convErr = errors.New("page must be at least 1")
		}
		return 0, false, convErr
	}
	return n, true, nil
}

func writeActivity(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", ContentTypeActivity)
	w.WriteHeader(code)
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(v)
}

func writeJRD(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", ContentTypeJRD)
	w.WriteHeader(code)
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(v)
}

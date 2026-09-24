package activitypub

import (
	"encoding/json"
	"html/template"
	"net/http"
	"strings"

	"github.com/fedishare/fedishare/internal/activitypub/web"
	"github.com/fedishare/fedishare/internal/activitystreams"
	"github.com/fedishare/fedishare/internal/files"
	"github.com/fedishare/fedishare/internal/version"
)

const htmlFileLimit = 50

var publicPages = template.Must(template.ParseFS(web.Templates, "templates/*.html"))

// ProfilePage is the browser view of a FediShare actor.
type ProfilePage struct {
	Title       string
	DisplayName string
	Acct        string
	Summary     string
	BrandName   string
	BrandURL    string
	JSONURL     string
	Files       []FileLink
	TotalFiles  int
	Offline     bool
	Version     string
}

// FileLink is one public file on the HTML profile or note page.
type FileLink struct {
	Name        string
	MIME        string
	Size        string
	NoteURL     string
	DownloadURL string
}

// NotePage is the browser view of one shared-file Note.
type NotePage struct {
	Title       string
	DisplayName string
	ProfileURL  string
	Name        string
	MIME        string
	Size        string
	DownloadURL string
	JSONURL     string
	Version     string
}

func (h *Handler) writeProfile(w http.ResponseWriter, r *http.Request) {
	paths := h.paths()
	display := strings.TrimSpace(h.src.DisplayName())
	if display == "" {
		display = h.src.Username()
	}
	acct := "@" + h.src.Username()
	if host := h.src.AcctHost(); host != "" {
		acct += "@" + host
	}
	page := ProfilePage{
		Title:       display + " · FediShare",
		DisplayName: display,
		Acct:        acct,
		Summary:     h.src.Summary(),
		BrandName:   activitystreams.BrandFieldName,
		BrandURL:    activitystreams.BrandFieldURL,
		JSONURL:     paths.Actor(),
		Version:     version.Version,
	}
	recs, total, err := h.src.ListPublicFiles(r.Context(), 0, htmlFileLimit)
	if err == nil {
		page.TotalFiles = total
		page.Files = fileLinks(paths, recs)
	}
	writePublicHTML(w, "profile.html", page)
}

func (h *Handler) writeNotePage(w http.ResponseWriter, rec files.Record) {
	paths := h.paths()
	display := strings.TrimSpace(h.src.DisplayName())
	if display == "" {
		display = h.src.Username()
	}
	name := rec.Filename
	if name == "" {
		name = "Shared file"
	}
	writePublicHTML(w, "note.html", NotePage{
		Title:       name + " · FediShare",
		DisplayName: display,
		ProfileURL:  paths.Actor(),
		Name:        name,
		MIME:        displayMIME(rec.MIMEType),
		Size:        activitystreams.FormatSize(rec.Size),
		DownloadURL: paths.Download(rec.ID),
		JSONURL:     paths.Note(rec.ID),
		Version:     version.Version,
	})
}

func fileLinks(paths activitystreams.Paths, recs []files.Record) []FileLink {
	out := make([]FileLink, 0, len(recs))
	for _, rec := range recs {
		name := rec.Filename
		if name == "" {
			name = rec.ID
		}
		out = append(out, FileLink{
			Name:        name,
			MIME:        displayMIME(rec.MIMEType),
			Size:        activitystreams.FormatSize(rec.Size),
			NoteURL:     paths.Note(rec.ID),
			DownloadURL: paths.Download(rec.ID),
		})
	}
	return out
}

// WriteOfflineProfileHTML renders a profile from a cached Actor JSON body
// when the desktop node is not connected.
func WriteOfflineProfileHTML(w http.ResponseWriter, actorJSON string) {
	page := ProfilePageFromActorJSON(actorJSON)
	page.Offline = true
	writePublicHTML(w, "profile.html", page)
}

// ProfilePageFromActorJSON extracts the public HTML fields from a Person document.
func ProfilePageFromActorJSON(raw string) ProfilePage {
	var actor struct {
		PreferredUsername string `json:"preferredUsername"`
		Name              string `json:"name"`
		Summary           string `json:"summary"`
		ID                string `json:"id"`
	}
	_ = json.Unmarshal([]byte(raw), &actor)
	display := strings.TrimSpace(actor.Name)
	if display == "" {
		display = actor.PreferredUsername
	}
	if display == "" {
		display = "FediShare"
	}
	acct := actor.PreferredUsername
	if acct != "" {
		acct = "@" + acct
		if host := hostFromActorID(actor.ID); host != "" {
			acct += "@" + host
		}
	}
	jsonURL := strings.TrimSpace(actor.ID)
	return ProfilePage{
		Title:       display + " · FediShare",
		DisplayName: display,
		Acct:        acct,
		Summary:     actor.Summary,
		BrandName:   activitystreams.BrandFieldName,
		BrandURL:    activitystreams.BrandFieldURL,
		JSONURL:     jsonURL,
		Version:     version.Version,
	}
}

func displayMIME(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexByte(s, ';'); i >= 0 {
		s = strings.TrimSpace(s[:i])
	}
	if s == "" {
		return "file"
	}
	return s
}

func hostFromActorID(id string) string {
	id = strings.TrimSpace(id)
	id = strings.TrimPrefix(id, "https://")
	id = strings.TrimPrefix(id, "http://")
	host, _, _ := strings.Cut(id, "/")
	return host
}

func writePublicHTML(w http.ResponseWriter, name string, data any) {
	var jsonURL string
	switch v := data.(type) {
	case ProfilePage:
		jsonURL = v.JSONURL
	case NotePage:
		jsonURL = v.JSONURL
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Vary", "Accept")
	w.Header().Set("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'; img-src 'self' data:; form-action 'none'; frame-ancestors 'none'")
	if jsonURL != "" {
		w.Header().Set("Link", `<`+jsonURL+`>; rel="alternate"; type="application/activity+json"`)
	}
	if err := publicPages.ExecuteTemplate(w, name, data); err != nil {
		http.Error(w, "Could not render the FediShare page.", http.StatusInternalServerError)
	}
}

// WriteUnavailableHTML is a short HTML page for public routes when the node
// is offline and no Actor cache is available.
func WriteUnavailableHTML(w http.ResponseWriter, code int, message string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Vary", "Accept")
	w.Header().Set("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'; img-src 'self' data:; form-action 'none'; frame-ancestors 'none'")
	w.WriteHeader(code)
	page := ProfilePage{
		Title:       "FediShare",
		DisplayName: "FediShare",
		Summary:     message,
		BrandName:   activitystreams.BrandFieldName,
		BrandURL:    activitystreams.BrandFieldURL,
		Offline:     true,
		Version:     version.Version,
	}
	_ = publicPages.ExecuteTemplate(w, "profile.html", page)
}

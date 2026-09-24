package activitystreams

import (
	"fmt"
	"html"

	"github.com/fedishare/fedishare/internal/files"
)

const (
	Context         = "https://www.w3.org/ns/activitystreams"
	SecurityContext = "https://w3id.org/security/v1"
	PublicAudience  = "https://www.w3.org/ns/activitystreams#Public"
)

// ActorContext is the JSON-LD context used on Person objects.
// The third term lets Mastodon-compatible clients render PropertyValue fields.
var ActorContext = []any{
	Context,
	SecurityContext,
	map[string]any{
		"schema":        "http://schema.org#",
		"PropertyValue": "schema:PropertyValue",
		"value":         "schema:value",
	},
}

// Document builds a generic ActivityStreams Document for a shared file.
// Extra FediShare fields live under a namespaced prefix so generic
// consumers can ignore them.
func Document(id, name, mediaType, downloadURL string, rec files.Record) map[string]any {
	doc := documentFields(id, name, mediaType, downloadURL, rec)
	doc["@context"] = Context
	return doc
}

func documentFields(id, name, mediaType, downloadURL string, rec files.Record) map[string]any {
	doc := map[string]any{
		"id":        id,
		"type":      "Document",
		"name":      name,
		"mediaType": mediaType,
		"url":       downloadURL,
	}
	if rec.Size > 0 {
		doc["fedishare:size"] = rec.Size
	}
	if rec.Hash.Digest != "" {
		doc["fedishare:hash"] = rec.Hash.URN()
		doc["fedishare:hashAlgorithm"] = rec.Hash.Algorithm
	}
	doc["fedishare:available"] = rec.Available
	if rec.Filename != "" {
		doc["fedishare:filename"] = rec.Filename
	}
	if !rec.IndexedAt.IsZero() {
		doc["published"] = rec.IndexedAt.UTC().Format("2006-01-02T15:04:05Z")
	}
	return doc
}

// NoteContent is HTML. ActivityStreams content is HTML by default; Friendica
// and WAFRN drop or hide plain-text Notes that Mastodon still renders.
func NoteContent(name, mediaType string, size int64, downloadURL string) string {
	if mediaType == "" {
		mediaType = "file"
	}
	safeName := html.EscapeString(name)
	safeType := html.EscapeString(mediaType)
	safeURL := html.EscapeString(downloadURL)
	return fmt.Sprintf(
		`<p>📄 %s<br>%s · %s</p><p>Download:<br><a href="%s" rel="nofollow noopener noreferrer">%s</a></p>`,
		safeName, safeType, formatSize(size), safeURL, safeURL,
	)
}

// NoteContentPlain is the text/plain source next to the HTML content.
func NoteContentPlain(name, mediaType string, size int64, downloadURL string) string {
	if mediaType == "" {
		mediaType = "file"
	}
	return fmt.Sprintf("📄 %s\n%s · %s\n\nDownload:\n%s", name, mediaType, formatSize(size), downloadURL)
}

// FormatSize is the human-readable size used on Notes and the public HTML pages.
func FormatSize(n int64) string {
	return formatSize(n)
}

func formatSize(n int64) string {
	if n < 1024 {
		return fmt.Sprintf("%d B", n)
	}
	val := float64(n)
	for _, u := range []string{"KB", "MB", "GB", "TB"} {
		val /= 1024
		if val < 1024 {
			return fmt.Sprintf("%.1f %s", val, u)
		}
	}
	return fmt.Sprintf("%.1f PB", val/1024)
}

package activitystreams

import (
	"fmt"

	"github.com/fedishare/fedishare/internal/files"
)

const (
	Context         = "https://www.w3.org/ns/activitystreams"
	SecurityContext = "https://w3id.org/security/v1"
	PublicAudience  = "https://www.w3.org/ns/activitystreams#Public"
)

// ActorContext is the JSON-LD context used on Person objects.
var ActorContext = []any{Context, SecurityContext}

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

// NoteContent is the human-readable Create body for Mastodon-compatible clients.
func NoteContent(name, mediaType string, size int64, downloadURL string) string {
	if mediaType == "" {
		mediaType = "file"
	}
	return fmt.Sprintf("📄 %s\n%s · %s\n\nDownload:\n%s", name, mediaType, formatSize(size), downloadURL)
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

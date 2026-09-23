package activitystreams

import (
	"time"

	"github.com/fedishare/fedishare/internal/files"
)

// FileCreate builds a Create activity whose object is a Note with a
// Document attachment. Generic Mastodon-compatible clients can render
// the Note; FediShare clients can read the namespaced Document fields.
func FileCreate(paths Paths, rec files.Record) map[string]any {
	published := rec.IndexedAt.UTC()
	if published.IsZero() {
		published = time.Now().UTC()
	}
	download := paths.Download(rec.ID)
	doc := documentFields(paths.File(rec.ID), rec.Filename, rec.MIMEType, download, rec)
	note := map[string]any{
		"id":           paths.Note(rec.ID),
		"type":         "Note",
		"attributedTo": paths.Actor(),
		"content":      NoteContent(rec.Filename, rec.MIMEType, rec.Size, download),
		"url":          download,
		"published":    published.Format(time.RFC3339),
		"to":           []string{PublicAudience},
		"attachment":   []any{doc},
	}
	return map[string]any{
		"@context":  Context,
		"id":        paths.Activity(rec.ID),
		"type":      "Create",
		"actor":     paths.Actor(),
		"published": published.Format(time.RFC3339),
		"to":        []string{PublicAudience},
		"object":    note,
	}
}

package activitystreams

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/fedishare/fedishare/internal/files"
)

func TestDocumentSerialization(t *testing.T) {
	rec := files.Record{
		Filename:   "manuale-amiga.pdf",
		Size:       17_600_000,
		Available:  true,
		Hash:       files.ContentID{Algorithm: files.AlgoSHA256, Digest: "abc"},
		Visibility: files.VisibilityPublic,
	}
	doc := Document(
		"https://nodes.example.org/users/alice/files/1",
		"manuale-amiga.pdf",
		"application/pdf",
		"https://nodes.example.org/users/alice/download/1",
		rec,
	)
	if doc["type"] != "Document" {
		t.Fatalf("type=%v", doc["type"])
	}
	if doc["@context"] != Context {
		t.Fatal("context")
	}
	raw, err := json.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), "fedishare:hash") {
		t.Fatalf("missing hash extension: %s", raw)
	}
	note := NoteContent(rec.Filename, "application/pdf", rec.Size, "https://example/dl")
	if !strings.Contains(note, "manuale-amiga.pdf") || !strings.Contains(note, "Download:") {
		t.Fatalf("note=%s", note)
	}
}

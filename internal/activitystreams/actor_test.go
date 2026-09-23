package activitystreams

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/fedishare/fedishare/internal/files"
)

func TestPersonSerialization(t *testing.T) {
	paths := NewPaths("https://nodes.example.org", "Alice")
	actor := Person(paths, "Alice", "Sharing files.", "-----BEGIN PUBLIC KEY-----\nMIIB\n-----END PUBLIC KEY-----\n")
	if actor.Type != "Person" {
		t.Fatalf("type=%s", actor.Type)
	}
	if actor.PreferredUsername != "alice" {
		t.Fatalf("username=%s", actor.PreferredUsername)
	}
	if actor.ID != "https://nodes.example.org/users/alice" {
		t.Fatalf("id=%s", actor.ID)
	}
	if actor.Inbox != actor.ID+"/inbox" || actor.Outbox != actor.ID+"/outbox" {
		t.Fatalf("collections inbox=%s outbox=%s", actor.Inbox, actor.Outbox)
	}
	if actor.PublicKey.ID != actor.ID+"#main-key" || actor.PublicKey.Owner != actor.ID {
		t.Fatalf("key=%+v", actor.PublicKey)
	}
	raw, err := json.Marshal(actor)
	if err != nil {
		t.Fatal(err)
	}
	s := string(raw)
	for _, need := range []string{`"preferredUsername":"alice"`, `"type":"Person"`, "publicKeyPem", Context, SecurityContext} {
		if !strings.Contains(s, need) {
			t.Fatalf("missing %q in %s", need, s)
		}
	}
}

func TestFileCreateNoteAndDocument(t *testing.T) {
	paths := NewPaths("https://nodes.example.org", "alice")
	rec := files.Record{
		ID:         "aabbccddeeff00112233445566778899",
		Filename:   "manuale-amiga.pdf",
		MIMEType:   "application/pdf",
		Size:       17_600_000,
		Available:  true,
		IndexedAt:  time.Date(2026, 9, 23, 12, 0, 0, 0, time.UTC),
		Hash:       files.ContentID{Algorithm: files.AlgoSHA256, Digest: "abc"},
		Visibility: files.VisibilityPublic,
	}
	create := FileCreate(paths, rec)
	if create["type"] != "Create" {
		t.Fatalf("type=%v", create["type"])
	}
	raw, err := json.Marshal(create)
	if err != nil {
		t.Fatal(err)
	}
	s := string(raw)
	for _, need := range []string{"Note", "Document", "manuale-amiga.pdf", "Download:", "fedishare:hash", PublicAudience} {
		if !strings.Contains(s, need) {
			t.Fatalf("missing %q in %s", need, s)
		}
	}
	if strings.Count(s, `"@context"`) != 1 {
		t.Fatalf("nested @context leaked: %s", s)
	}
}

func TestCollectionPaginationLinks(t *testing.T) {
	col := Collection("https://nodes.example.org/users/alice/outbox", 45, 20)
	if col.Type != "OrderedCollection" || col.TotalItems != 45 {
		t.Fatalf("%+v", col)
	}
	if col.First != "https://nodes.example.org/users/alice/outbox?page=1" {
		t.Fatalf("first=%s", col.First)
	}
	if col.Last != "https://nodes.example.org/users/alice/outbox?page=3" {
		t.Fatalf("last=%s", col.Last)
	}
	empty := Collection("https://example/outbox", 0, 20)
	if empty.First != "" || empty.Last != "" {
		t.Fatalf("empty collection should omit pages: %+v", empty)
	}
}

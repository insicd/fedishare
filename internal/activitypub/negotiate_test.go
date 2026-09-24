package activitypub

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestWantsHTML(t *testing.T) {
	htmlReq := httptest.NewRequest(http.MethodGet, "/users/alice", nil)
	htmlReq.Header.Set("Accept", "text/html,application/xhtml+xml;q=0.9,*/*;q=0.8")
	if !WantsHTML(htmlReq) {
		t.Fatal("browser accept should want HTML")
	}
	jsonReq := httptest.NewRequest(http.MethodGet, "/users/alice", nil)
	jsonReq.Header.Set("Accept", `application/activity+json, application/ld+json; profile="https://www.w3.org/ns/activitystreams"`)
	if WantsHTML(jsonReq) {
		t.Fatal("federation accept should stay JSON")
	}
	empty := httptest.NewRequest(http.MethodGet, "/users/alice", nil)
	if WantsHTML(empty) {
		t.Fatal("missing accept should stay JSON")
	}
}

func TestActorAndNoteHTML(t *testing.T) {
	h := mount(testSrc())
	req := httptest.NewRequest(http.MethodGet, "http://127.0.0.1:17890/users/alice", nil)
	req.Host = "127.0.0.1:17890"
	req.Header.Set("Accept", "text/html")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatal(rec.Body.String())
	}
	if !strings.Contains(rec.Header().Get("Content-Type"), "text/html") {
		t.Fatalf("ctype=%s", rec.Header().Get("Content-Type"))
	}
	body := rec.Body.String()
	for _, need := range []string{"Alice", "@alice@nodes.example.org", "Sharing files.", "hello.txt", "My Web", "https://nodes.example.org/users/alice", "Fedishare", "github.com/insicd/fedishare"} {
		if !strings.Contains(body, need) {
			t.Fatalf("missing %q in %s", need, body)
		}
	}

	note := httptest.NewRequest(http.MethodGet, "http://127.0.0.1:17890/users/alice/notes/aabbccddeeff00112233445566778899", nil)
	note.Host = "127.0.0.1:17890"
	note.Header.Set("Accept", "text/html")
	noteRec := httptest.NewRecorder()
	h.ServeHTTP(noteRec, note)
	if noteRec.Code != 200 || !strings.Contains(noteRec.Body.String(), "hello.txt") {
		t.Fatalf("note page %d %s", noteRec.Code, noteRec.Body.String())
	}
}

func TestProfilePageFromActorJSON(t *testing.T) {
	page := ProfilePageFromActorJSON(`{"id":"https://nodes.example.org/users/alice","preferredUsername":"alice","name":"Alice","summary":"hi"}`)
	if page.DisplayName != "Alice" || page.Acct != "@alice@nodes.example.org" || page.Summary != "hi" {
		t.Fatalf("%+v", page)
	}
	if page.WebName != "My Web" || page.WebURL != "https://nodes.example.org/users/alice" {
		t.Fatalf("web field %+v", page)
	}
}

package activitypub

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/fedishare/fedishare/internal/files"
)

type fakeSource struct {
	configured bool
	username   string
	display    string
	summary    string
	pem        string
	base       string
	host       string
	files      []files.Record
}

func (f fakeSource) Configured() bool    { return f.configured }
func (f fakeSource) Username() string    { return f.username }
func (f fakeSource) DisplayName() string { return f.display }
func (f fakeSource) Summary() string     { return f.summary }
func (f fakeSource) PublicKeyPEM() (string, error) {
	return f.pem, nil
}
func (f fakeSource) PublicBase() string { return f.base }
func (f fakeSource) AcctHost() string   { return f.host }
func (f fakeSource) ListPublicFiles(_ context.Context, offset, limit int) ([]files.Record, int, error) {
	if offset > len(f.files) {
		return nil, len(f.files), nil
	}
	end := offset + limit
	if end > len(f.files) {
		end = len(f.files)
	}
	return f.files[offset:end], len(f.files), nil
}
func (f fakeSource) GetPublicFile(_ context.Context, id string) (files.Record, error) {
	for _, rec := range f.files {
		if rec.ID == id {
			return rec, nil
		}
	}
	return files.Record{}, files.ErrUnavailable
}

func testSrc() fakeSource {
	return fakeSource{
		configured: true,
		username:   "alice",
		display:    "Alice",
		summary:    "Sharing files.",
		pem:        "-----BEGIN PUBLIC KEY-----\nMIIB\n-----END PUBLIC KEY-----\n",
		base:       "https://nodes.example.org",
		host:       "nodes.example.org",
		files: []files.Record{{
			ID:        "aabbccddeeff00112233445566778899",
			Filename:  "hello.txt",
			MIMEType:  "text/plain",
			Size:      5,
			Available: true,
			IndexedAt: time.Date(2026, 9, 23, 12, 0, 0, 0, time.UTC),
			Hash:      files.ContentID{Algorithm: files.AlgoSHA256, Digest: "abc"},
		}},
	}
}

func mount(src Source) http.Handler {
	mux := http.NewServeMux()
	Mount(mux, src)
	return mux
}

func TestWebFingerAndActor(t *testing.T) {
	h := mount(testSrc())
	req := httptest.NewRequest(http.MethodGet, "http://127.0.0.1:17890/.well-known/webfinger?resource=acct:alice@nodes.example.org", nil)
	req.Host = "127.0.0.1:17890"
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatalf("webfinger %d %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Header().Get("Content-Type"), "application/jrd+json") {
		t.Fatalf("ctype=%s", rec.Header().Get("Content-Type"))
	}
	var jrd JRD
	if err := json.Unmarshal(rec.Body.Bytes(), &jrd); err != nil {
		t.Fatal(err)
	}
	if jrd.Subject != "acct:alice@nodes.example.org" {
		t.Fatalf("subject=%s", jrd.Subject)
	}
	if len(jrd.Links) == 0 || jrd.Links[0].Href != "https://nodes.example.org/users/alice" {
		t.Fatalf("links=%+v", jrd.Links)
	}

	unknown := httptest.NewRequest(http.MethodGet, "http://127.0.0.1:17890/.well-known/webfinger?resource=acct:eve@nodes.example.org", nil)
	unknown.Host = "127.0.0.1:17890"
	bad := httptest.NewRecorder()
	h.ServeHTTP(bad, unknown)
	if bad.Code != 404 {
		t.Fatalf("unknown acct %d", bad.Code)
	}

	actorReq := httptest.NewRequest(http.MethodGet, "http://127.0.0.1:17890/users/alice", nil)
	actorReq.Host = "127.0.0.1:17890"
	actorRec := httptest.NewRecorder()
	h.ServeHTTP(actorRec, actorReq)
	if actorRec.Code != 200 {
		t.Fatalf("actor %d %s", actorRec.Code, actorRec.Body.String())
	}
	if !strings.Contains(actorRec.Header().Get("Content-Type"), "application/activity+json") {
		t.Fatalf("ctype=%s", actorRec.Header().Get("Content-Type"))
	}
	var actor map[string]any
	if err := json.Unmarshal(actorRec.Body.Bytes(), &actor); err != nil {
		t.Fatal(err)
	}
	if actor["type"] != "Person" || actor["preferredUsername"] != "alice" {
		t.Fatalf("%v", actor)
	}
	rawActor, _ := json.Marshal(actor)
	if !strings.Contains(string(rawActor), "Fedishare") || !strings.Contains(string(rawActor), "github.com/insicd/fedishare") {
		t.Fatalf("missing brand field: %s", rawActor)
	}
	if actor["inbox"] != "https://nodes.example.org/users/alice/inbox" {
		t.Fatalf("inbox=%v", actor["inbox"])
	}
	pk, _ := actor["publicKey"].(map[string]any)
	if pk == nil || !strings.Contains(pk["publicKeyPem"].(string), "BEGIN PUBLIC KEY") {
		t.Fatalf("publicKey=%v", pk)
	}

	wrong := httptest.NewRequest(http.MethodGet, "http://127.0.0.1:17890/users/bob", nil)
	wrong.Host = "127.0.0.1:17890"
	wrongRec := httptest.NewRecorder()
	h.ServeHTTP(wrongRec, wrong)
	if wrongRec.Code != 404 {
		t.Fatalf("wrong user %d", wrongRec.Code)
	}
}

func TestOutboxPagesAndFileObject(t *testing.T) {
	h := mount(testSrc())
	colReq := httptest.NewRequest(http.MethodGet, "http://127.0.0.1:17890/users/alice/outbox", nil)
	colReq.Host = "127.0.0.1:17890"
	colRec := httptest.NewRecorder()
	h.ServeHTTP(colRec, colReq)
	if colRec.Code != 200 {
		t.Fatal(colRec.Body.String())
	}
	var col map[string]any
	if err := json.Unmarshal(colRec.Body.Bytes(), &col); err != nil {
		t.Fatal(err)
	}
	if col["type"] != "OrderedCollection" || col["totalItems"] != float64(1) {
		t.Fatalf("%v", col)
	}

	pageReq := httptest.NewRequest(http.MethodGet, "http://127.0.0.1:17890/users/alice/outbox?page=1", nil)
	pageReq.Host = "127.0.0.1:17890"
	pageRec := httptest.NewRecorder()
	h.ServeHTTP(pageRec, pageReq)
	var page map[string]any
	if err := json.Unmarshal(pageRec.Body.Bytes(), &page); err != nil {
		t.Fatal(err)
	}
	if page["type"] != "OrderedCollectionPage" {
		t.Fatalf("%v", page)
	}
	items, _ := page["orderedItems"].([]any)
	if len(items) != 1 {
		t.Fatalf("items=%v", page["orderedItems"])
	}
	create, _ := items[0].(map[string]any)
	if create["type"] != "Create" {
		t.Fatalf("create=%v", create)
	}

	fileReq := httptest.NewRequest(http.MethodGet, "http://127.0.0.1:17890/users/alice/files/aabbccddeeff00112233445566778899", nil)
	fileReq.Host = "127.0.0.1:17890"
	fileRec := httptest.NewRecorder()
	h.ServeHTTP(fileRec, fileReq)
	if fileRec.Code != 200 {
		t.Fatal(fileRec.Body.String())
	}
	if !strings.Contains(fileRec.Body.String(), `"type":"Document"`) {
		t.Fatalf("doc=%s", fileRec.Body.String())
	}

	empty := httptest.NewRequest(http.MethodGet, "http://127.0.0.1:17890/users/alice/followers", nil)
	empty.Host = "127.0.0.1:17890"
	emptyRec := httptest.NewRecorder()
	h.ServeHTTP(emptyRec, empty)
	if emptyRec.Code != 200 || !strings.Contains(emptyRec.Body.String(), `"totalItems":0`) {
		t.Fatalf("followers=%s", emptyRec.Body.String())
	}
}

func TestUnconfiguredReturns404(t *testing.T) {
	src := testSrc()
	src.configured = false
	h := mount(src)
	req := httptest.NewRequest(http.MethodGet, "http://127.0.0.1:17890/.well-known/webfinger?resource=acct:alice@nodes.example.org", nil)
	req.Host = "127.0.0.1:17890"
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != 404 {
		t.Fatalf("code=%d", rec.Code)
	}
}

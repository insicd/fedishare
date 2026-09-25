package network

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestFetchDirectoryAndLookup(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /.well-known/fedishare-network", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(Directory{
			Type:    TypeDirectory,
			Gateway: "http://" + r.Host,
			Actors: []Actor{{
				Username: "alice",
				Acct:     "alice@" + r.Host,
				Name:     "Alice",
				URL:      "http://" + r.Host + "/users/alice",
				Online:   true,
			}},
		})
	})
	mux.HandleFunc("GET /.well-known/webfinger", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("resource") != "acct:alice@"+r.Host {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/jrd+json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"subject": "acct:alice@" + r.Host,
			"links": []map[string]string{{
				"rel":  "self",
				"type": "application/activity+json",
				"href": "http://" + r.Host + "/users/alice",
			}},
		})
	})
	mux.HandleFunc("GET /users/alice", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/activity+json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":                "http://" + r.Host + "/users/alice",
			"preferredUsername": "alice",
			"name":              "Alice",
			"summary":           "Sharing files.",
		})
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	c := NewClient(true)
	dir, err := FetchDirectory(context.Background(), c, srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	if len(dir.Actors) != 1 || dir.Actors[0].Name != "Alice" || !dir.Actors[0].Online {
		t.Fatalf("%+v", dir)
	}
	looked, err := Resolve(context.Background(), c, "alice@"+httptestHost(srv.URL), "")
	if err != nil {
		t.Fatal(err)
	}
	if len(looked.Actors) != 1 || looked.Actors[0].Summary != "Sharing files." {
		t.Fatalf("%+v", looked)
	}
}

func httptestHost(raw string) string {
	return HostFromBase(raw)
}

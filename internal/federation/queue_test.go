package federation

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func mustKey(t *testing.T) *rsa.PrivateKey {
	t.Helper()
	k, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	return k
}

func TestQueueRetriesThenGivesUpOnPermanent(t *testing.T) {
	store := testDB(t)
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	payload := []byte(`{"type":"Accept","id":"https://a/1"}`)
	if err := store.Enqueue(context.Background(), srv.URL+"/inbox", "https://a/1", payload); err != nil {
		t.Fatal(err)
	}
	w := NewWorker()
	w.Store = store
	w.Fetcher = NewFetcher(true)
	w.KeyID = func() string { return "https://nodes.example.org/users/alice#main-key" }
	w.Private = func() (*rsa.PrivateKey, error) {
		t.Helper()
		return mustKey(t), nil
	}
	w.Log = nil
	job, err := store.ClaimDue(context.Background(), 1)
	if err != nil || len(job) != 1 {
		t.Fatalf("jobs=%v err=%v", job, err)
	}
	w.deliver(context.Background(), job[0])
	if hits.Load() != 1 {
		t.Fatalf("hits=%d", hits.Load())
	}
	var status string
	err = store.db.SQL().QueryRow(`SELECT status FROM federation_queue WHERE activity_id = ?`, "https://a/1").Scan(&status)
	if err != nil || status != StatusFailed {
		t.Fatalf("status=%s err=%v", status, err)
	}
}

func TestQueueDeliversOn2xx(t *testing.T) {
	store := testDB(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.Copy(io.Discard, r.Body)
		if r.Header.Get("Signature") == "" || r.Header.Get("Digest") == "" {
			t.Error("missing signature headers")
		}
		w.WriteHeader(http.StatusAccepted)
	}))
	defer srv.Close()
	payload := []byte(`{"type":"Accept","id":"https://a/2"}`)
	if err := store.Enqueue(context.Background(), srv.URL+"/inbox", "https://a/2", payload); err != nil {
		t.Fatal(err)
	}
	w := NewWorker()
	w.Store = store
	w.Fetcher = NewFetcher(true)
	w.KeyID = func() string { return "https://nodes.example.org/users/alice#main-key" }
	w.Private = func() (*rsa.PrivateKey, error) { return mustKey(t), nil }
	jobs, _ := store.ClaimDue(context.Background(), 1)
	w.deliver(context.Background(), jobs[0])
	var status string
	_ = store.db.SQL().QueryRow(`SELECT status FROM federation_queue WHERE activity_id = ?`, "https://a/2").Scan(&status)
	if status != StatusDelivered {
		t.Fatalf("status=%s", status)
	}
}

func TestBackoffGrows(t *testing.T) {
	if backoff(1) != time.Second || backoff(2) != 2*time.Second {
		t.Fatalf("backoff %v %v", backoff(1), backoff(2))
	}
	if backoff(20) != time.Hour {
		t.Fatalf("cap %v", backoff(20))
	}
}

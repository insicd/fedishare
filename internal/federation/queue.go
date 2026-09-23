package federation

import (
	"bytes"
	"context"
	"crypto/rsa"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/fedishare/fedishare/internal/httpsig"
	"github.com/fedishare/fedishare/internal/security"
	"github.com/fedishare/fedishare/internal/version"
)

// Worker delivers queued activities to remote inboxes.
type Worker struct {
	Store    *Store
	Fetcher  *Fetcher
	KeyID    func() string
	Private  func() (*rsa.PrivateKey, error)
	Paused   func() bool
	Status   func(pending int, lastErr string, ok bool)
	Log      *slog.Logger
	Interval time.Duration
	Kick     chan struct{}

	done chan struct{}
}

func NewWorker() *Worker {
	return &Worker{
		Interval: 2 * time.Second,
		Kick:     make(chan struct{}, 1),
		done:     make(chan struct{}),
	}
}

func (w *Worker) Notify() {
	if w.Kick == nil {
		return
	}
	select {
	case w.Kick <- struct{}{}:
	default:
	}
}

func (w *Worker) Wait() {
	if w == nil || w.done == nil {
		return
	}
	select {
	case <-w.done:
	case <-time.After(3 * time.Second):
	}
}

func (w *Worker) Run(ctx context.Context) {
	if w.done == nil {
		w.done = make(chan struct{})
	}
	defer close(w.done)
	if w.Log == nil {
		w.Log = slog.Default()
	}
	if w.Interval <= 0 {
		w.Interval = 2 * time.Second
	}
	tick := time.NewTicker(w.Interval)
	defer tick.Stop()
	w.tick(ctx)
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
			w.tick(ctx)
		case <-w.Kick:
			w.tick(ctx)
		}
	}
}

func (w *Worker) tick(ctx context.Context) {
	jobs, err := w.Store.ClaimDue(ctx, 8)
	if err != nil {
		w.Log.Warn("federation queue", "err", err)
		return
	}
	for _, job := range jobs {
		if ctx.Err() != nil {
			return
		}
		if w.Paused != nil && w.Paused() && isFileActivity(job.Payload) {
			continue
		}
		w.deliver(ctx, job)
	}
	if w.Status != nil {
		n, _ := w.Store.PendingCount(ctx)
		w.Status(n, "", true)
	}
}

func (w *Worker) deliver(ctx context.Context, job Job) {
	if w.Log == nil {
		w.Log = slog.Default()
	}
	host := security.HostOnly(job.InboxURL)
	priv, err := w.Private()
	if err != nil {
		w.fail(ctx, job, err, false)
		return
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, job.InboxURL, bytes.NewReader(job.Payload))
	if err != nil {
		w.fail(ctx, job, err, false)
		return
	}
	req.Header.Set("Content-Type", "application/activity+json")
	req.Header.Set("Accept", "application/activity+json")
	req.Header.Set("User-Agent", "FediShare/"+version.Version)
	if err := httpsig.SignRequest(req, w.KeyID(), priv, job.Payload); err != nil {
		w.fail(ctx, job, err, false)
		return
	}
	res, err := w.Fetcher.Client.Do(req)
	if err != nil {
		w.fail(ctx, job, err, true)
		return
	}
	_, _ = io.Copy(io.Discard, io.LimitReader(res.Body, 4096))
	res.Body.Close()
	if res.StatusCode >= 200 && res.StatusCode < 300 {
		if err := w.Store.MarkDelivered(ctx, job.ID); err != nil {
			w.Log.Warn("mark delivered", "err", err)
		}
		if w.Status != nil {
			n, _ := w.Store.PendingCount(ctx)
			w.Status(n, "", true)
		}
		w.Log.Info("federation delivered", "host", host, "status", res.StatusCode)
		return
	}
	permanent := res.StatusCode == 400 || res.StatusCode == 401 || res.StatusCode == 403 ||
		res.StatusCode == 404 || res.StatusCode == 410 || res.StatusCode == 422
	w.fail(ctx, job, fmt.Errorf("HTTP %d", res.StatusCode), !permanent)
}

func (w *Worker) fail(ctx context.Context, job Job, err error, retry bool) {
	attempts := job.Attempts + 1
	status := StatusPending
	next := time.Now().UTC().Add(backoff(attempts))
	msg := err.Error()
	if !retry || attempts >= job.MaxAttempts {
		status = StatusFailed
	}
	if err := w.Store.MarkAttempt(ctx, job.ID, attempts, next, msg, status); err != nil {
		w.Log.Warn("queue attempt", "err", err)
	}
	if w.Status != nil {
		n, _ := w.Store.PendingCount(ctx)
		w.Status(n, msg, false)
	}
	w.Log.Info("federation delivery failed", "host", security.HostOnly(job.InboxURL), "err", err, "status", status)
}

func backoff(attempt int) time.Duration {
	if attempt < 1 {
		attempt = 1
	}
	d := time.Second << (attempt - 1)
	if d > time.Hour {
		d = time.Hour
	}
	return d
}

func isFileActivity(payload []byte) bool {
	t := payloadType(payload)
	return t == "Create" || t == "Update" || t == "Delete"
}

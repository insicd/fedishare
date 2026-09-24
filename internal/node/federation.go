package node

import (
	"context"
	"crypto/rsa"
	"net/http"
	"strings"

	"github.com/fedishare/fedishare/internal/activitypub"
	"github.com/fedishare/fedishare/internal/activitystreams"
	"github.com/fedishare/fedishare/internal/federation"
	"github.com/fedishare/fedishare/internal/status"
)

func (n *Node) allowPrivateFederation() bool {
	if n.allowLocal {
		return true
	}
	if n.cfg != nil && strings.TrimSpace(n.cfg.GatewayURL) == "" {
		return true
	}
	return false
}

func (n *Node) startFederationLocked() {
	if n.db == nil {
		return
	}
	allow := n.allowPrivateFederation()
	store := federation.NewStore(n.db)
	fetch := federation.NewFetcher(allow)
	fetch.KeyID = func() string { return n.publicPaths().KeyID() }
	fetch.Private = func() (*rsa.PrivateKey, error) { return n.keys.PrivateKey() }
	n.fedStore = store
	n.fedFetch = fetch
	n.publisher = &federation.Publisher{
		Store:  store,
		Paths:  n.publicPaths,
		Paused: n.Paused,
		Enqueue: func(ctx context.Context, inboxURL, activityID string, payload []byte) error {
			err := store.Enqueue(ctx, inboxURL, activityID, payload)
			if n.worker != nil {
				n.worker.Notify()
			}
			return err
		},
		Log: n.log,
	}
	n.inbox = &federation.Inbox{
		Store:   store,
		Fetcher: fetch,
		Paths:   n.publicPaths,
		Enqueue: func(ctx context.Context, inboxURL, activityID string, payload []byte) error {
			err := store.Enqueue(ctx, inboxURL, activityID, payload)
			if n.worker != nil {
				n.worker.Notify()
			}
			return err
		},
		Log: n.log,
	}
	w := federation.NewWorker()
	w.Store = store
	w.Fetcher = fetch
	w.KeyID = func() string { return n.publicPaths().KeyID() }
	w.Private = func() (*rsa.PrivateKey, error) { return n.keys.PrivateKey() }
	w.Paused = n.Paused
	w.Log = n.log
	w.Status = func(pending int, lastErr string, ok bool) {
		_ = n.status.Update(func(snap *status.Snapshot) error {
			snap.PendingFederationJobs = pending
			if lastErr != "" {
				snap.LastFederationError = lastErr
			}
			return nil
		})
	}
	n.worker = w
	if n.indexer != nil {
		n.indexer.SetHook(n.publisher.OnIndexChange)
	}
}

func (n *Node) publicPaths() activitystreams.Paths {
	return activitystreams.NewPaths(n.PublicBase(), n.Username())
}

func (n *Node) ProcessInbox(ctx context.Context, r *http.Request, body []byte) error {
	n.mu.Lock()
	in := n.inbox
	n.mu.Unlock()
	if in == nil {
		return federation.ErrBadActivity
	}
	return in.Process(ctx, r, body)
}

func (n *Node) CountFollowers(ctx context.Context) (int, error) {
	if n.fedStore == nil {
		return 0, nil
	}
	return n.fedStore.CountFollowers(ctx)
}

func (n *Node) ListFollowerIDs(ctx context.Context, offset, limit int) ([]string, int, error) {
	if n.fedStore == nil {
		return nil, 0, nil
	}
	list, total, err := n.fedStore.ListFollowers(ctx, offset, limit)
	if err != nil {
		return nil, 0, err
	}
	out := make([]string, 0, len(list))
	for _, f := range list {
		out = append(out, f.ActorID)
	}
	return out, total, nil
}

func (n *Node) CountFollowing(ctx context.Context) (int, error) {
	if n.fedStore == nil {
		return 0, nil
	}
	_, total, err := n.fedStore.ListFollowingIDs(ctx, 0, 1)
	return total, err
}

func (n *Node) ListFollowingIDs(ctx context.Context, offset, limit int) ([]string, int, error) {
	if n.fedStore == nil {
		return nil, 0, nil
	}
	return n.fedStore.ListFollowingIDs(ctx, offset, limit)
}

func (n *Node) FollowerList() ([]federation.Follower, error) {
	if n.fedStore == nil {
		return nil, nil
	}
	list, _, err := n.fedStore.ListFollowers(context.Background(), 0, 200)
	return list, err
}

func (n *Node) Block(ctx context.Context, target string) error {
	if n.fedStore == nil || target == "" {
		return nil
	}
	kind := federation.BlockActor
	if !strings.Contains(target, "://") {
		kind = federation.BlockDomain
	}
	if err := n.fedStore.Block(ctx, target, kind); err != nil {
		return err
	}
	if kind == federation.BlockActor {
		_ = n.fedStore.RemoveFollower(ctx, target)
	}
	return nil
}

func (n *Node) Unblock(ctx context.Context, target string) error {
	if n.fedStore == nil {
		return nil
	}
	return n.fedStore.Unblock(ctx, target)
}

func (n *Node) federationActivityList() ([]activitypub.OutboxItem, error) {
	if n.fedStore == nil {
		return nil, nil
	}
	acts, err := n.fedStore.ListActivities(context.Background(), 50)
	if err != nil || len(acts) == 0 {
		return nil, err
	}
	out := make([]activitypub.OutboxItem, 0, len(acts))
	for _, a := range acts {
		item := activitypub.OutboxItem{
			ID:        a.ID,
			Type:      a.Type,
			Published: a.PublishedAt,
		}
		if a.FileID != "" {
			item.Name = a.FileID
			item.ObjectURL = "/users/" + n.Username() + "/files/" + a.FileID
			item.DownloadURL = "/users/" + n.Username() + "/download/" + a.FileID
		}
		out = append(out, item)
	}
	return out, nil
}

// compile-time check
var _ activitypub.Social = (*Node)(nil)

package federation

import (
	"context"
	"log/slog"
	"strconv"
	"time"

	"github.com/fedishare/fedishare/internal/activitystreams"
	"github.com/fedishare/fedishare/internal/indexer"
)

// Publisher turns local index changes into queued deliveries.
type Publisher struct {
	Store   *Store
	Paths   func() activitystreams.Paths
	Paused  func() bool
	Enqueue func(ctx context.Context, inboxURL, activityID string, payload []byte) error
	Log     *slog.Logger
}

func (p *Publisher) OnIndexChange(ctx context.Context, ch indexer.Change) {
	if p.Paused != nil && p.Paused() {
		return
	}
	if p.Log == nil {
		p.Log = slog.Default()
	}
	paths := p.Paths()
	var (
		activity map[string]any
		id       string
		typ      string
	)
	switch ch.Kind {
	case indexer.ChangeCreate:
		activity = activitystreams.FileCreate(paths, ch.Rec)
		id = asString(activity["id"])
		typ = "Create"
	case indexer.ChangeUpdate:
		id = paths.Activity(ch.Rec.ID) + "/updates/" + strconv.FormatInt(time.Now().Unix(), 10)
		activity = activitystreams.FileUpdate(paths, ch.Rec, id)
		typ = "Update"
	case indexer.ChangeDelete:
		id = paths.Activity(ch.Rec.ID) + "/deletes/" + strconv.FormatInt(time.Now().Unix(), 10)
		activity = activitystreams.FileDelete(paths, ch.Rec, id)
		typ = "Delete"
	default:
		return
	}
	payload, err := encodeJSON(activity)
	if err != nil {
		p.Log.Warn("encode activity", "err", err)
		return
	}
	if err := p.Store.SaveActivity(ctx, Activity{
		ID:          id,
		Type:        typ,
		FileID:      ch.Rec.ID,
		PublishedAt: nowRFC3339(),
		Payload:     string(payload),
	}); err != nil {
		p.Log.Warn("save activity", "err", err)
		return
	}
	followers, err := p.Store.AcceptedInboxes(ctx)
	if err != nil {
		p.Log.Warn("list followers", "err", err)
		return
	}
	seen := map[string]struct{}{}
	for _, f := range followers {
		inbox := f.InboxURL
		if f.SharedInbox != "" {
			inbox = f.SharedInbox
		}
		if inbox == "" {
			continue
		}
		if _, ok := seen[inbox]; ok {
			continue
		}
		seen[inbox] = struct{}{}
		if p.Enqueue != nil {
			if err := p.Enqueue(ctx, inbox, id, payload); err != nil {
				p.Log.Warn("enqueue delivery", "err", err, "host", f.Host)
			}
		}
	}
}

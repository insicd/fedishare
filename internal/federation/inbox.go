package federation

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/hex"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/fedishare/fedishare/internal/activitystreams"
	"github.com/fedishare/fedishare/internal/httpsig"
	"github.com/fedishare/fedishare/internal/security"
)

// Inbox processes signed ActivityPub activities addressed to this node.
type Inbox struct {
	Store   *Store
	Fetcher *Fetcher
	Paths   func() activitystreams.Paths
	Enqueue func(ctx context.Context, inboxURL, activityID string, payload []byte) error
	Log     *slog.Logger
}

func (in *Inbox) Process(ctx context.Context, r *http.Request, body []byte) error {
	if in.Log == nil {
		in.Log = slog.Default()
	}
	if len(body) == 0 || len(body) > maxInboxBody {
		return ErrBadActivity
	}

	var fetched RemoteActor
	params, err := httpsig.VerifyRequest(r, body, func(keyID string) (*rsa.PublicKey, error) {
		if !isHTTPURL(actorFromKeyID(keyID)) {
			return nil, ErrUnauthorized
		}
		pub, actor, ferr := in.Fetcher.PublicKey(ctx, keyID)
		if ferr != nil {
			return nil, ferr
		}
		fetched = actor
		return pub, nil
	})
	if err != nil {
		in.Log.Info("inbox signature rejected", "err", err, "host", security.HostOnly("https://"+r.Host))
		return ErrUnauthorized
	}

	seen, err := in.Store.SeenSignature(ctx, httpsig.HashSignature(params.Signature))
	if err != nil {
		return err
	}
	if seen {
		return ErrReplay
	}

	m, err := decodeMap(body)
	if err != nil {
		return ErrBadActivity
	}
	typ := typeOf(m["type"])
	actorID := actorIRI(m["actor"])
	if actorID == "" || !isHTTPURL(actorID) {
		return ErrBadActivity
	}
	if fetched.ID != "" && !sameIRI(actorID, fetched.ID) {
		return ErrUnauthorized
	}
	host := hostOf(actorID)
	blocked, err := in.Store.IsBlocked(ctx, actorID, host)
	if err != nil {
		return err
	}
	if blocked {
		return ErrForbidden
	}

	if err := in.Store.RememberSignature(ctx, httpsig.HashSignature(params.Signature)); err != nil {
		return err
	}

	switch typ {
	case "Follow":
		return in.handleFollow(ctx, m, actorID, fetched)
	case "Undo":
		return in.handleUndo(ctx, m, actorID)
	case "Accept":
		return in.handleAccept(ctx, m, actorID)
	case "Reject":
		return in.handleReject(ctx, m, actorID)
	case "Delete":
		return in.handleDelete(ctx, m, actorID)
	default:
		in.Log.Debug("inbox ignored type", "type", typ)
		return nil
	}
}

func (in *Inbox) handleFollow(ctx context.Context, m map[string]any, actorID string, remote RemoteActor) error {
	paths := in.Paths()
	object := objectID(m["object"])
	if object != "" && !sameIRI(object, paths.Actor()) {
		return ErrBadActivity
	}
	if remote.ID == "" || remote.Inbox == "" {
		fetched, err := in.Fetcher.FetchActor(ctx, actorID)
		if err != nil {
			return err
		}
		remote = fetched
	}
	followID := asString(m["id"])
	if followID == "" {
		followID = actorID + "#follows/" + paths.Username
	}
	f := Follower{
		ActorID:     remote.ID,
		InboxURL:    remote.Inbox,
		SharedInbox: remote.SharedInbox,
		Username:    remote.Username,
		Host:        hostOf(remote.ID),
		Accepted:    true,
	}
	if err := in.Store.UpsertFollower(ctx, f); err != nil {
		return err
	}
	acceptID := paths.Actor() + "/activities/accepts/" + newOpaque()
	payload, err := encodeJSON(activitystreams.AcceptFollow(paths, acceptID, followID, remote.ID))
	if err != nil {
		return err
	}
	if err := in.Store.SaveActivity(ctx, Activity{
		ID:          acceptID,
		Type:        "Accept",
		PublishedAt: nowRFC3339(),
		Payload:     string(payload),
	}); err != nil {
		return err
	}
	if in.Enqueue != nil {
		return in.Enqueue(ctx, remote.Inbox, acceptID, payload)
	}
	return nil
}

func (in *Inbox) handleUndo(ctx context.Context, m map[string]any, actorID string) error {
	obj := asMap(m["object"])
	innerType := typeOf(m["object"])
	if obj != nil {
		innerType = typeOf(obj["type"])
	}
	if !strings.EqualFold(innerType, "Follow") && innerType != "" {
		return nil
	}
	innerActor := actorID
	if obj != nil {
		if a := actorIRI(obj["actor"]); a != "" {
			innerActor = a
		}
	}
	if !sameIRI(innerActor, actorID) {
		return ErrBadActivity
	}
	return in.Store.RemoveFollower(ctx, actorID)
}

func (in *Inbox) handleAccept(ctx context.Context, m map[string]any, actorID string) error {
	ok, err := in.Store.HasOutgoingFollow(ctx, actorID)
	if err != nil || !ok {
		return err
	}
	return in.Store.FollowingAccepted(ctx, actorID, true)
}

func (in *Inbox) handleReject(ctx context.Context, m map[string]any, actorID string) error {
	ok, err := in.Store.HasOutgoingFollow(ctx, actorID)
	if err != nil || !ok {
		return err
	}
	return in.Store.FollowingAccepted(ctx, actorID, false)
}

func (in *Inbox) handleDelete(ctx context.Context, m map[string]any, actorID string) error {
	obj := objectID(m["object"])
	if obj == "" || sameIRI(obj, actorID) {
		return in.Store.RemoveFollower(ctx, actorID)
	}
	if inner := asMap(m["object"]); inner != nil {
		if typeOf(inner["type"]) == "Tombstone" && sameIRI(objectID(inner), actorID) {
			return in.Store.RemoveFollower(ctx, actorID)
		}
	}
	return nil
}

func newOpaque() string {
	var b [12]byte
	if _, err := rand.Read(b[:]); err != nil {
		return fmt.Sprintf("%d", len(b))
	}
	return hex.EncodeToString(b[:])
}

func nowRFC3339() string {
	return time.Now().UTC().Format(time.RFC3339)
}

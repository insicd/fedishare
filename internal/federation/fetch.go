package federation

import (
	"context"
	"crypto/rsa"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/fedishare/fedishare/internal/crypto"
	"github.com/fedishare/fedishare/internal/security"
)

type keyEntry struct {
	pub   *rsa.PublicKey
	exp   time.Time
	actor RemoteActor
}

// Fetcher retrieves remote Actors with SSRF protections.
type Fetcher struct {
	Client *http.Client
	Policy security.Policy

	mu    sync.Mutex
	cache map[string]keyEntry
}

func NewFetcher(allowPrivate bool) *Fetcher {
	p := security.Policy{AllowPrivate: allowPrivate}
	return &Fetcher{
		Client: p.Client(12 * time.Second),
		Policy: p,
		cache:  make(map[string]keyEntry),
	}
}

func (f *Fetcher) FetchActor(ctx context.Context, actorURL string) (RemoteActor, error) {
	if err := f.Policy.CheckURL(actorURL); err != nil {
		return RemoteActor{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, actorURL, nil)
	if err != nil {
		return RemoteActor{}, err
	}
	req.Header.Set("Accept", "application/activity+json, application/ld+json")
	res, err := f.Client.Do(req)
	if err != nil {
		return RemoteActor{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return RemoteActor{}, fmt.Errorf("fetch actor: HTTP %d", res.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(res.Body, maxActorBody+1))
	if err != nil {
		return RemoteActor{}, err
	}
	if len(body) > maxActorBody {
		return RemoteActor{}, fmt.Errorf("actor document too large")
	}
	m, err := decodeMap(body)
	if err != nil {
		return RemoteActor{}, fmt.Errorf("actor is not JSON")
	}
	id := asString(m["id"])
	if id == "" {
		id = actorURL
	}
	if !isHTTPURL(id) || !isHTTPURL(asString(m["inbox"])) {
		return RemoteActor{}, ErrBadActivity
	}
	actor := RemoteActor{
		ID:       id,
		Inbox:    asString(m["inbox"]),
		Username: asString(m["preferredUsername"]),
	}
	if ep := asMap(m["endpoints"]); ep != nil {
		actor.SharedInbox = asString(ep["sharedInbox"])
	}
	if pk := asMap(m["publicKey"]); pk != nil {
		actor.PublicKeyID = asString(pk["id"])
		actor.PublicKeyPEM = asString(pk["publicKeyPem"])
	}
	if actor.PublicKeyPEM == "" {
		return RemoteActor{}, fmt.Errorf("actor has no publicKeyPem")
	}
	if err := f.Policy.CheckURL(actor.Inbox); err != nil {
		return RemoteActor{}, err
	}
	if actor.SharedInbox != "" {
		if err := f.Policy.CheckURL(actor.SharedInbox); err != nil {
			actor.SharedInbox = ""
		}
	}
	return actor, nil
}

func (f *Fetcher) PublicKey(ctx context.Context, keyID string) (*rsa.PublicKey, RemoteActor, error) {
	f.mu.Lock()
	if e, ok := f.cache[keyID]; ok && time.Now().Before(e.exp) {
		f.mu.Unlock()
		return e.pub, e.actor, nil
	}
	f.mu.Unlock()

	actorURL := actorFromKeyID(keyID)
	actor, err := f.FetchActor(ctx, actorURL)
	if err != nil {
		return nil, RemoteActor{}, err
	}
	if actor.PublicKeyID != "" && !sameIRI(actor.PublicKeyID, keyID) && !strings.HasPrefix(keyID, actor.ID) {
		return nil, RemoteActor{}, fmt.Errorf("keyId does not belong to actor")
	}
	pub, err := crypto.ParsePublicKeyPEM([]byte(actor.PublicKeyPEM))
	if err != nil {
		return nil, RemoteActor{}, err
	}
	f.mu.Lock()
	f.cache[keyID] = keyEntry{pub: pub, actor: actor, exp: time.Now().Add(10 * time.Minute)}
	f.mu.Unlock()
	return pub, actor, nil
}

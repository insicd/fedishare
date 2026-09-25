package network

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/fedishare/fedishare/internal/apperr"
	"github.com/fedishare/fedishare/internal/security"
	"github.com/fedishare/fedishare/internal/version"
)

const maxBody = 256 << 10

// Client fetches public FediShare directories and remote profiles.
type Client struct {
	Policy security.Policy
	HTTP   *http.Client
}

// NewClient builds a client that obeys the federation SSRF policy.
func NewClient(allowPrivate bool) *Client {
	p := security.Policy{AllowPrivate: allowPrivate}
	return &Client{
		Policy: p,
		HTTP:   p.Client(12 * time.Second),
	}
}

// FetchDirectory loads /.well-known/fedishare-network from a gateway.
func FetchDirectory(ctx context.Context, c *Client, gatewayURL string) (Directory, error) {
	base := strings.TrimRight(strings.TrimSpace(gatewayURL), "/")
	if base == "" {
		return Directory{}, apperr.New(apperr.KindInvalidConfig, "Set a gateway URL in Settings to see other FediShare users.")
	}
	raw, err := c.get(ctx, base+WellKnownPath, "application/json")
	if err != nil {
		return Directory{}, apperr.Wrap(apperr.KindTemporaryNetwork, "Could not reach that FediShare gateway.", err)
	}
	var dir Directory
	if err := json.Unmarshal(raw, &dir); err != nil {
		return Directory{}, apperr.New(apperr.KindFederationReject, "This host is not a FediShare gateway.")
	}
	if dir.Type != "" && dir.Type != TypeDirectory {
		return Directory{}, apperr.New(apperr.KindFederationReject, "This host is not a FediShare gateway.")
	}
	dir.Type = TypeDirectory
	if dir.Gateway == "" {
		dir.Gateway = base
	}
	if dir.Actors == nil {
		dir.Actors = []Actor{}
	}
	if len(dir.Actors) > maxActors {
		dir.Actors = dir.Actors[:maxActors]
	}
	return dir, nil
}

// Resolve looks up a gateway directory, an @user@host, or an actor URL.
func Resolve(ctx context.Context, c *Client, query, homeGateway string) (Directory, error) {
	kind, value := Classify(query)
	switch kind {
	case KindEmpty:
		if strings.TrimSpace(homeGateway) == "" {
			return Directory{
				Type:   TypeDirectory,
				Actors: []Actor{},
				Hint:   "Set a gateway URL in Settings to see other FediShare users.",
			}, nil
		}
		return FetchDirectory(ctx, c, homeGateway)
	case KindGateway:
		dir, err := FetchDirectory(ctx, c, value)
		if err != nil {
			return Directory{}, err
		}
		dir.Query = query
		return dir, nil
	case KindAcct:
		if !strings.Contains(value, "@") {
			host := HostFromBase(homeGateway)
			if host == "" {
				return Directory{}, apperr.New(apperr.KindInvalidConfig, "Enter a full @user@host address or a gateway URL.")
			}
			value = value + "@" + host
		}
		actor, err := LookupAcct(ctx, c, value)
		if err != nil {
			return Directory{}, err
		}
		return Directory{
			Type:    TypeDirectory,
			Gateway: "https://" + HostFromBase(actor.URL),
			Query:   query,
			Actors:  []Actor{actor},
		}, nil
	case KindActor:
		actor, err := LookupActorURL(ctx, c, value)
		if err != nil {
			return Directory{}, err
		}
		return Directory{
			Type:    TypeDirectory,
			Gateway: "https://" + HostFromBase(actor.URL),
			Query:   query,
			Actors:  []Actor{actor},
		}, nil
	default:
		return Directory{}, apperr.New(apperr.KindInvalidConfig, "Enter a gateway URL or an @user@host address.")
	}
}

// LookupAcct resolves user@host through WebFinger, then the actor document.
func LookupAcct(ctx context.Context, c *Client, acct string) (Actor, error) {
	user, host, ok := splitAcct(acct)
	if !ok {
		return Actor{}, apperr.New(apperr.KindInvalidConfig, "Enter a gateway URL or an @user@host address.")
	}
	resource := "acct:" + user + "@" + host
	q := url.Values{"resource": {resource}}
	var last error
	for _, scheme := range []string{"https", "http"} {
		wfURL := scheme + "://" + host + "/.well-known/webfinger?" + q.Encode()
		raw, err := c.get(ctx, wfURL, "application/jrd+json, application/json")
		if err != nil {
			last = err
			continue
		}
		href := actorHrefFromJRD(raw)
		if href == "" {
			href = scheme + "://" + host + "/users/" + user
		}
		actor, err := LookupActorURL(ctx, c, href)
		if err != nil {
			last = err
			continue
		}
		if actor.Username == "" {
			actor.Username = user
		}
		if actor.Acct == "" {
			actor.Acct = user + "@" + host
		}
		return actor, nil
	}
	if last != nil {
		return Actor{}, apperr.Wrap(apperr.KindTemporaryNetwork, "Could not find that account.", last)
	}
	return Actor{}, apperr.New(apperr.KindFederationReject, "Could not find that account.")
}

// LookupActorURL loads a public Actor document for display.
func LookupActorURL(ctx context.Context, c *Client, actorURL string) (Actor, error) {
	raw, err := c.get(ctx, actorURL, "application/activity+json, application/ld+json, application/json")
	if err != nil {
		return Actor{}, apperr.Wrap(apperr.KindTemporaryNetwork, "Could not load that profile.", err)
	}
	base := actorURL
	if u, err := url.Parse(actorURL); err == nil {
		base = u.Scheme + "://" + u.Host
	}
	username := ""
	if u, err := url.Parse(actorURL); err == nil {
		path := strings.TrimSuffix(u.Path, "/")
		if strings.HasPrefix(path, "/users/") {
			username = strings.TrimPrefix(path, "/users/")
		}
	}
	a := ActorFromDocument(username, base, string(raw), false)
	if a.URL == "" {
		a.URL = actorURL
	}
	if a.Username == "" || a.Name == "" && a.URL == "" {
		return Actor{}, apperr.New(apperr.KindFederationReject, "That address is not a public profile.")
	}
	return a, nil
}

func actorHrefFromJRD(raw []byte) string {
	var jrd struct {
		Links []struct {
			Rel  string `json:"rel"`
			Type string `json:"type"`
			Href string `json:"href"`
		} `json:"links"`
	}
	if json.Unmarshal(raw, &jrd) != nil {
		return ""
	}
	var html string
	for _, l := range jrd.Links {
		if l.Href == "" {
			continue
		}
		if l.Rel == "self" && strings.Contains(l.Type, "activity") {
			return l.Href
		}
		if l.Rel == "http://webfinger.net/rel/profile-page" {
			html = l.Href
		}
	}
	return html
}

func (c *Client) get(ctx context.Context, rawURL, accept string) ([]byte, error) {
	if c == nil {
		return nil, fmt.Errorf("network client is missing")
	}
	if err := c.Policy.CheckURL(rawURL); err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", accept)
	req.Header.Set("User-Agent", "FediShare/"+version.Version)
	res, err := c.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	body, err := io.ReadAll(io.LimitReader(res.Body, maxBody+1))
	if err != nil {
		return nil, err
	}
	if len(body) > maxBody {
		return nil, fmt.Errorf("response too large")
	}
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d", res.StatusCode)
	}
	return body, nil
}

func unmarshalActor(raw string, dest any) error {
	return json.Unmarshal([]byte(raw), dest)
}

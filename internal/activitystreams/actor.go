package activitystreams

import "html"

const (
	// WebFieldName is the fixed profile field that points at this actor's
	// public browser URL.
	WebFieldName = "My Web"
	// BrandFieldName is the fixed Mastodon-style profile field on every actor.
	BrandFieldName = "Fedishare"
	// BrandFieldURL is the project homepage shown in that field.
	BrandFieldURL = "https://github.com/insicd/fedishare"
)

// PublicKey is the W3C security vocabulary publicKey on a Person.
type PublicKey struct {
	ID           string `json:"id"`
	Owner        string `json:"owner"`
	PublicKeyPem string `json:"publicKeyPem"`
}

// PropertyValue is a Mastodon-compatible profile metadata field.
type PropertyValue struct {
	Type  string `json:"type"`
	Name  string `json:"name"`
	Value string `json:"value"`
}

// WebAttachment is the non-customizable public profile URL field.
func WebAttachment(actorURL string) PropertyValue {
	return linkAttachment(WebFieldName, actorURL)
}

// BrandAttachment is the non-customizable Fedishare profile field.
func BrandAttachment() PropertyValue {
	return linkAttachment(BrandFieldName, BrandFieldURL)
}

func linkAttachment(name, rawURL string) PropertyValue {
	safe := html.EscapeString(rawURL)
	return PropertyValue{
		Type:  "PropertyValue",
		Name:  name,
		Value: `<a href="` + safe + `" rel="me nofollow noopener noreferrer" target="_blank">` + safe + `</a>`,
	}
}

// Actor is a standards-compliant ActivityPub Person.
type Actor struct {
	Context                   any             `json:"@context"`
	ID                        string          `json:"id"`
	Type                      string          `json:"type"`
	PreferredUsername         string          `json:"preferredUsername"`
	Name                      string          `json:"name,omitempty"`
	Summary                   string          `json:"summary,omitempty"`
	Inbox                     string          `json:"inbox"`
	Outbox                    string          `json:"outbox"`
	Followers                 string          `json:"followers"`
	Following                 string          `json:"following"`
	URL                       string          `json:"url"`
	Attachment                []PropertyValue `json:"attachment,omitempty"`
	PublicKey                 PublicKey       `json:"publicKey"`
	ManuallyApprovesFollowers bool            `json:"manuallyApprovesFollowers"`
	Discoverable              bool            `json:"discoverable"`
}

// Person builds the local user's Actor document.
func Person(paths Paths, displayName, summary, publicKeyPEM string) Actor {
	return Actor{
		Context:           ActorContext,
		ID:                paths.Actor(),
		Type:              "Person",
		PreferredUsername: paths.Username,
		Name:              displayName,
		Summary:           summary,
		Inbox:             paths.Inbox(),
		Outbox:            paths.Outbox(),
		Followers:         paths.Followers(),
		Following:         paths.Following(),
		URL:               paths.Actor(),
		Attachment:        []PropertyValue{WebAttachment(paths.Actor()), BrandAttachment()},
		PublicKey: PublicKey{
			ID:           paths.KeyID(),
			Owner:        paths.Actor(),
			PublicKeyPem: publicKeyPEM,
		},
		ManuallyApprovesFollowers: false,
		Discoverable:              true,
	}
}

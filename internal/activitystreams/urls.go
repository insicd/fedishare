package activitystreams

import (
	"fmt"
	"strings"
)

// Paths builds the public ActivityPub URLs for one actor.
// Base is the gateway origin (or the local dashboard origin in development).
type Paths struct {
	Base     string
	Username string
}

func NewPaths(base, username string) Paths {
	return Paths{
		Base:     strings.TrimRight(strings.TrimSpace(base), "/"),
		Username: strings.ToLower(strings.TrimSpace(username)),
	}
}

func (p Paths) Actor() string     { return p.Base + "/users/" + p.Username }
func (p Paths) Inbox() string     { return p.Actor() + "/inbox" }
func (p Paths) Outbox() string    { return p.Actor() + "/outbox" }
func (p Paths) Followers() string { return p.Actor() + "/followers" }
func (p Paths) Following() string { return p.Actor() + "/following" }

func (p Paths) File(id string) string     { return p.Actor() + "/files/" + id }
func (p Paths) Download(id string) string { return p.Actor() + "/download/" + id }
func (p Paths) Activity(id string) string { return p.Actor() + "/activities/" + id }
func (p Paths) Note(id string) string     { return p.Actor() + "/notes/" + id }
func (p Paths) OutboxPage(n int) string   { return fmt.Sprintf("%s?page=%d", p.Outbox(), n) }
func (p Paths) KeyID() string             { return p.Actor() + "#main-key" }
func (p Paths) Acct(host string) string   { return "acct:" + p.Username + "@" + host }

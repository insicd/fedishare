package federation

import (
	"errors"
	"time"
)

var (
	ErrUnauthorized = errors.New("unsigned or invalid HTTP signature")
	ErrForbidden    = errors.New("actor or domain is blocked")
	ErrBadActivity  = errors.New("invalid activity")
	ErrReplay       = errors.New("replayed signature")
)

const (
	StatusPending   = "pending"
	StatusDelivered = "delivered"
	StatusFailed    = "failed"

	BlockActor  = "actor"
	BlockDomain = "domain"

	maxInboxBody = 1 << 20
	maxActorBody = 256 << 10
	maxAttempts  = 12
	replayRetain = 24 * time.Hour
	defaultPage  = 20
)

// Follower is a remote actor accepted as a follower.
type Follower struct {
	ActorID     string `json:"actor_id"`
	InboxURL    string `json:"inbox_url"`
	SharedInbox string `json:"shared_inbox,omitempty"`
	Username    string `json:"username,omitempty"`
	Host        string `json:"host"`
	Accepted    bool   `json:"accepted"`
	CreatedAt   string `json:"created_at"`
}

// Job is one durable outbound delivery.
type Job struct {
	ID          int64
	InboxURL    string
	ActivityID  string
	Payload     []byte
	Attempts    int
	MaxAttempts int
	NextAttempt time.Time
	LastError   string
	Status      string
}

// Activity is a persisted outgoing ActivityPub document.
type Activity struct {
	ID          string `json:"id"`
	Type        string `json:"type"`
	FileID      string `json:"file_id,omitempty"`
	PublishedAt string `json:"published"`
	Payload     string `json:"payload,omitempty"`
}

// RemoteActor is the subset of an Actor we need to follow or deliver.
type RemoteActor struct {
	ID           string
	Inbox        string
	SharedInbox  string
	Username     string
	PublicKeyID  string
	PublicKeyPEM string
}

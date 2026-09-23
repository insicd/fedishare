package activitypub

import (
	"context"
	"net/http"

	"github.com/fedishare/fedishare/internal/files"
)

const (
	ContentTypeActivity = "application/activity+json; charset=utf-8"
	ContentTypeJRD      = "application/jrd+json; charset=utf-8"

	maxResourceLen = 512
	pageSize       = 20
)

// Source is the local node's view of the single ActivityPub actor it owns.
type Source interface {
	Configured() bool
	Username() string
	DisplayName() string
	Summary() string
	PublicKeyPEM() (string, error)
	PublicBase() string
	AcctHost() string
	ListPublicFiles(ctx context.Context, offset, limit int) ([]files.Record, int, error)
	GetPublicFile(ctx context.Context, id string) (files.Record, error)
}

// Social is implemented by a node that processes Follows and lists followers.
type Social interface {
	ProcessInbox(ctx context.Context, r *http.Request, body []byte) error
	CountFollowers(ctx context.Context) (int, error)
	ListFollowerIDs(ctx context.Context, offset, limit int) ([]string, int, error)
	CountFollowing(ctx context.Context) (int, error)
	ListFollowingIDs(ctx context.Context, offset, limit int) ([]string, int, error)
}

// OutboxItem is the dashboard view of one Create in the outbox.
type OutboxItem struct {
	ID          string `json:"id"`
	Type        string `json:"type"`
	Name        string `json:"name"`
	Published   string `json:"published"`
	ObjectURL   string `json:"object_url"`
	DownloadURL string `json:"download_url"`
}

package httpserver

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/fedishare/fedishare/internal/activitypub"
	"github.com/fedishare/fedishare/internal/config"
	"github.com/fedishare/fedishare/internal/federation"
	"github.com/fedishare/fedishare/internal/files"
	"github.com/fedishare/fedishare/internal/network"
	"github.com/fedishare/fedishare/internal/status"
)

// Backend is the local dashboard's view of the node.
type Backend interface {
	Status() *status.Service
	Config() *config.Config
	Home() string
	DashboardURL() string
	NodeID() string
	ApplySetup(ctx context.Context, in SetupRequest) error
	UpdateSettings(ctx context.Context, in SettingsRequest) error
	Pause(ctx context.Context) error
	Resume(ctx context.Context) error
	Rescan(ctx context.Context) error
	FileList() ([]files.Entry, error)
	FileHandler() http.Handler
	ActivityList() ([]activitypub.OutboxItem, error)
	FollowerList() ([]federation.Follower, error)
	Block(ctx context.Context, target string) error
	Unblock(ctx context.Context, target string) error
}

// Network is implemented by a node that can list other FediShare actors.
type Network interface {
	FetchNetwork(ctx context.Context, query string) (network.Directory, error)
}

// Profiles is implemented by a multi-actor desktop host.
type Profiles interface {
	Profiles() []config.ProfileInfo
	SelectProfile(id string) error
	CreateProfile(ctx context.Context, in SetupRequest) error
}

// PublicRouter dispatches public ActivityPub URLs to the matching local actor.
type PublicRouter interface {
	ServePublic(w http.ResponseWriter, r *http.Request)
}

type SetupRequest struct {
	DisplayName    string `json:"display_name"`
	Username       string `json:"username"`
	ShareDirectory string `json:"share_directory"`
	GatewayURL     string `json:"gateway_url"`
	Summary        string `json:"summary"`
}

type SettingsRequest struct {
	DisplayName            *string `json:"display_name"`
	Username               *string `json:"username"`
	Summary                *string `json:"summary"`
	ShareDirectory         *string `json:"share_directory"`
	GatewayURL             *string `json:"gateway_url"`
	LocalPort              *int    `json:"local_port"`
	MaxConcurrentDownloads *int    `json:"max_concurrent_downloads"`
	BandwidthLimitBPS      *int64  `json:"bandwidth_limit_bps"`
	LogLevel               *string `json:"log_level"`
	StartAtLogin           *bool   `json:"start_at_login"`
}

type Server struct {
	backend   Backend
	log       *slog.Logger
	csrfToken string
}

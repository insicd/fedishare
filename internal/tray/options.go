package tray

import (
	"log/slog"

	"github.com/fedishare/fedishare/internal/status"
)

// Options wire the tray to the running node without importing it.
type Options struct {
	Status          *status.Service
	Log             *slog.Logger
	OpenDashboard   func()
	OpenShareFolder func()
	Rescan          func()
	CopyAddress     func()
	Pause           func()
	Resume          func()
	OpenSettings    func()
	OpenAbout       func()
	OnQuit          func()
}

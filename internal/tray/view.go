package tray

import (
	"fmt"

	"github.com/fedishare/fedishare/internal/status"
)

func Tooltip(snap status.Snapshot) string {
	switch snap.State {
	case status.StateIndexing:
		return fmt.Sprintf("FediShare — Indexing\n%d / %d files", snap.IndexingDone, max(snap.IndexingTotal, snap.IndexedFiles))
	case status.StatePaused:
		return "FediShare — Sharing paused"
	case status.StateError:
		if snap.Error != "" {
			return "FediShare — Error\n" + snap.Error
		}
		return "FediShare — Error"
	case status.StateOffline, status.StateConnecting:
		if snap.Message == "Setup required" {
			return "FediShare — Setup required"
		}
		return "FediShare — Offline\nGateway disconnected"
	case status.StateStarting, status.StateShuttingDown:
		return "FediShare — " + snap.State.Label()
	default:
		return fmt.Sprintf("FediShare — %s\n%d files shared", snap.State.Label(), snap.IndexedFiles)
	}
}

func StatusLine(snap status.Snapshot) string {
	return "Status: " + snap.State.Label()
}

func IdentityLine(snap status.Snapshot) string {
	if snap.Identity == "" {
		return "Identity: not set"
	}
	return "Identity: " + snap.Identity
}

func FilesLine(snap status.Snapshot) string {
	return fmt.Sprintf("Shared files: %d", snap.IndexedFiles)
}

func PauseTitle(snap status.Snapshot) string {
	if snap.State == status.StatePaused {
		return "Resume sharing"
	}
	return "Pause sharing"
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

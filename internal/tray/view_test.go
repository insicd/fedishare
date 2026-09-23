package tray

import (
	"strings"
	"testing"

	"github.com/fedishare/fedishare/internal/status"
)

func TestTooltipStates(t *testing.T) {
	if !strings.Contains(Tooltip(status.Snapshot{State: status.StateOnline, IndexedFiles: 42}), "42 files shared") {
		t.Fatal("online")
	}
	if !strings.Contains(Tooltip(status.Snapshot{State: status.StateOffline, Message: "Gateway not connected"}), "Offline") {
		t.Fatal("offline")
	}
	if !strings.Contains(Tooltip(status.Snapshot{State: status.StatePaused}), "paused") {
		t.Fatal("paused")
	}
	if PauseTitle(status.Snapshot{State: status.StatePaused}) != "Resume sharing" {
		t.Fatal("resume title")
	}
	if IdentityLine(status.Snapshot{Identity: "@alice@nodes.example.org"}) != "Identity: @alice@nodes.example.org" {
		t.Fatal("identity")
	}
}

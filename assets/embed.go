package assets

import (
	"embed"

	"github.com/fedishare/fedishare/internal/status"
)

//go:embed *.png *.ico
var FS embed.FS

func TrayIcon(state status.State) []byte {
	name := "tray-offline.png"
	switch state {
	case status.StateStarting, status.StateConnecting, status.StateIndexing:
		name = "tray-starting.png"
	case status.StateOnline:
		name = "tray-online.png"
	case status.StatePaused:
		name = "tray-paused.png"
	case status.StateError:
		name = "tray-error.png"
	case status.StateOffline, status.StateShuttingDown:
		name = "tray-offline.png"
	}
	data, err := FS.ReadFile(name)
	if err != nil {
		data, _ = FS.ReadFile("icon.png")
	}
	return data
}

func AppIconPNG() []byte {
	data, _ := FS.ReadFile("icon.png")
	return data
}

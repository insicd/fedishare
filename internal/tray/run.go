//go:build cgo

package tray

import (
	"context"
	"log/slog"

	"fyne.io/systray"
	"github.com/fedishare/fedishare/assets"
	"github.com/fedishare/fedishare/internal/status"
)

// Run blocks on the platform tray event loop until Quit.
func Run(ctx context.Context, opts Options) {
	if opts.Log == nil {
		opts.Log = slog.Default()
	}
	go func() {
		<-ctx.Done()
		systray.Quit()
	}()
	systray.Run(func() { onReady(opts) }, func() {
		if opts.OnQuit != nil {
			opts.OnQuit()
		}
	})
}

func onReady(opts Options) {
	systray.SetTitle("")
	systray.SetTooltip("FediShare")
	applyIcon(opts.Status.Snapshot())

	systray.AddMenuItem("FediShare", "FediShare").Disable()
	systray.AddSeparator()
	statusItem := systray.AddMenuItem("Status: Starting", "")
	statusItem.Disable()
	identityItem := systray.AddMenuItem("Identity: not set", "")
	identityItem.Disable()
	filesItem := systray.AddMenuItem("Shared files: 0", "")
	filesItem.Disable()
	systray.AddSeparator()
	openItem := systray.AddMenuItem("Open FediShare", "Open the local dashboard")
	folderItem := systray.AddMenuItem("Open shared folder", "Reveal the shared folder")
	rescanItem := systray.AddMenuItem("Rescan files", "Refresh the local file list")
	copyItem := systray.AddMenuItem("Copy Fediverse address", "")
	pauseItem := systray.AddMenuItem("Pause sharing", "")
	systray.AddSeparator()
	settingsItem := systray.AddMenuItem("Settings", "")
	aboutItem := systray.AddMenuItem("About FediShare", "")
	quitItem := systray.AddMenuItem("Quit", "Quit FediShare")

	applyMenu := func(snap status.Snapshot) {
		statusItem.SetTitle(StatusLine(snap))
		identityItem.SetTitle(IdentityLine(snap))
		filesItem.SetTitle(FilesLine(snap))
		pauseItem.SetTitle(PauseTitle(snap))
		systray.SetTooltip(Tooltip(snap))
		applyIcon(snap)
		configured := snap.Identity != "" || snap.ShareDirectory != ""
		folderItem.Disable()
		rescanItem.Disable()
		copyItem.Disable()
		pauseItem.Disable()
		if configured {
			folderItem.Enable()
			rescanItem.Enable()
			copyItem.Enable()
			pauseItem.Enable()
		}
	}
	applyMenu(opts.Status.Snapshot())

	watch, unsub := opts.Status.Watch()
	go func() {
		defer unsub()
		for snap := range watch {
			applyMenu(snap)
		}
	}()

	go func() {
		for {
			select {
			case <-openItem.ClickedCh:
				go call(opts.OpenDashboard)
			case <-folderItem.ClickedCh:
				go call(opts.OpenShareFolder)
			case <-rescanItem.ClickedCh:
				go call(opts.Rescan)
			case <-copyItem.ClickedCh:
				go call(opts.CopyAddress)
			case <-pauseItem.ClickedCh:
				go func() {
					if opts.Status.Snapshot().State == status.StatePaused {
						call(opts.Resume)
					} else {
						call(opts.Pause)
					}
				}()
			case <-settingsItem.ClickedCh:
				go call(opts.OpenSettings)
			case <-aboutItem.ClickedCh:
				go call(opts.OpenAbout)
			case <-quitItem.ClickedCh:
				systray.Quit()
				return
			}
		}
	}()
}

func call(fn func()) {
	if fn != nil {
		fn()
	}
}

func applyIcon(snap status.Snapshot) {
	systray.SetIcon(assets.TrayIcon(snap.State))
}

package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/fedishare/fedishare/internal/config"
	"github.com/fedishare/fedishare/internal/desktop"
	"github.com/fedishare/fedishare/internal/logging"
	"github.com/fedishare/fedishare/internal/node"
	"github.com/fedishare/fedishare/internal/tray"
	"github.com/fedishare/fedishare/internal/version"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "%s: %v\n", version.AppName, err)
		os.Exit(1)
	}
}

func run(args []string) error {
	fs := flag.NewFlagSet("fedishare", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	dataDir := fs.String("data-dir", "", "override the application data directory")
	logLevel := fs.String("log-level", "", "override log level (debug, info, warn, error)")
	noTray := fs.Bool("no-tray", false, "do not show the system tray icon")
	noOpen := fs.Bool("no-open", false, "do not open the dashboard in a browser")
	showVersion := fs.Bool("version", false, "print version and exit")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *showVersion {
		fmt.Printf("%s %s\n", version.AppName, version.Version)
		return nil
	}

	home, err := config.ResolveHome(*dataDir)
	if err != nil {
		return err
	}
	if err := config.EnsureHome(home); err != nil {
		return err
	}

	if err := config.MigrateLegacyHome(home); err != nil {
		return err
	}
	app, err := config.LoadApp(home)
	if err != nil {
		return err
	}
	if *logLevel != "" {
		app.LogLevel = *logLevel
		_ = app.Save(home)
	}

	logger, closeLog, err := logging.Setup(app.LogLevel, config.LogPath(home))
	if err != nil {
		return err
	}
	defer func() { _ = closeLog() }()

	h, err := node.NewHost(home, logger)
	if err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := h.Start(ctx); err != nil {
		return err
	}

	url := h.DashboardURL()
	snap := h.Status().Snapshot()
	fmt.Fprintf(os.Stderr, "%s %s — %s (%s)\n", version.AppName, version.Version, snap.State.Label(), snap.Message)
	fmt.Fprintf(os.Stderr, "Dashboard: %s\n", url)
	if !*noOpen {
		if err := desktop.OpenURL(url); err != nil {
			logger.Warn("could not open dashboard", "err", err)
		}
	}

	if *noTray {
		<-ctx.Done()
	} else {
		tray.Run(ctx, tray.Options{
			Status: h.Status(),
			Log:    logger,
			OpenDashboard: func() {
				_ = desktop.OpenURL(h.DashboardURL())
			},
			OpenShareFolder: func() {
				_ = desktop.OpenPath(h.Config().ShareDirectory)
			},
			Rescan: func() {
				_ = h.Rescan(context.Background())
			},
			CopyAddress: func() {
				_ = desktop.CopyText(h.Config().FediverseAddress())
			},
			Pause: func() {
				_ = h.Pause(context.Background())
			},
			Resume: func() {
				_ = h.Resume(context.Background())
			},
			OpenSettings: func() {
				_ = desktop.OpenURL(h.DashboardURL() + "/#settings")
			},
			OpenAbout: func() {
				_ = desktop.OpenURL(h.DashboardURL() + "/#about")
			},
			OnQuit: stop,
		})
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return h.Shutdown(shutdownCtx)
}

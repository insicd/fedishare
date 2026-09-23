package main

import (
	"context"
	"flag"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/fedishare/fedishare/internal/gateway"
	"github.com/fedishare/fedishare/internal/logging"
	"github.com/fedishare/fedishare/internal/version"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "fedishare-gateway: %v\n", err)
		os.Exit(1)
	}
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func firstEnv(keys ...string) string {
	for _, key := range keys {
		if v := os.Getenv(key); v != "" {
			return v
		}
	}
	return ""
}

func run(args []string) error {
	fs := flag.NewFlagSet("fedishare-gateway", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	listen := fs.String("listen", envOr("FEDISHARE_LISTEN", "127.0.0.1:8080"), "address to listen on")
	dataDir := fs.String("data-dir", firstEnv("FEDISHARE_DATA_DIR"), "directory for gateway.db (required)")
	publicURL := fs.String("public-url", firstEnv("FEDISHARE_PUBLIC_URL", "CLOUDRON_APP_ORIGIN"), "public origin, e.g. https://nodes.example.org")
	tlsCert := fs.String("tls-cert", firstEnv("FEDISHARE_TLS_CERT"), "TLS certificate file (optional)")
	tlsKey := fs.String("tls-key", firstEnv("FEDISHARE_TLS_KEY"), "TLS private key file (optional)")
	logLevel := fs.String("log-level", envOr("FEDISHARE_LOG_LEVEL", "info"), "log level (debug, info, warn, error)")
	showVersion := fs.Bool("version", false, "print version and exit")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *showVersion {
		fmt.Printf("%s-gateway %s\n", version.AppName, version.Version)
		return nil
	}
	if *dataDir == "" {
		return fmt.Errorf("--data-dir is required")
	}
	abs, err := filepath.Abs(*dataDir)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(abs, 0o700); err != nil {
		return err
	}
	logger, closeLog, err := logging.Setup(*logLevel, filepath.Join(abs, "gateway.log"))
	if err != nil {
		return err
	}
	defer func() { _ = closeLog() }()

	store, err := gateway.Open(filepath.Join(abs, "gateway.db"))
	if err != nil {
		return err
	}
	defer store.Close()

	srv := gateway.New(store, *publicURL, logger)
	httpSrv := gateway.HTTPServer(srv.Handler())
	httpSrv.Addr = *listen

	ln, err := net.Listen("tcp", *listen)
	if err != nil {
		return err
	}
	logger.Info("gateway listening",
		"addr", ln.Addr().String(),
		"public_url", *publicURL,
		"tls", *tlsCert != "" && *tlsKey != "",
	)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	errc := make(chan error, 1)
	go func() {
		if *tlsCert != "" || *tlsKey != "" {
			if *tlsCert == "" || *tlsKey == "" {
				errc <- fmt.Errorf("--tls-cert and --tls-key must be set together")
				return
			}
			errc <- httpSrv.ServeTLS(ln, *tlsCert, *tlsKey)
			return
		}
		errc <- httpSrv.Serve(ln)
	}()

	select {
	case <-ctx.Done():
	case err := <-errc:
		if err != nil && err != http.ErrServerClosed {
			return err
		}
	}
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return httpSrv.Shutdown(shutdownCtx)
}

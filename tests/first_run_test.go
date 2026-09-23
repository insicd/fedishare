package tests

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/fedishare/fedishare/internal/config"
	"github.com/fedishare/fedishare/internal/node"
)

func TestFirstRunWritesConfigThenWizardPersists(t *testing.T) {
	home := t.TempDir()
	cfg := config.Default()
	cfg.LocalPort = 0
	n, err := node.New(node.Options{
		Home:   home,
		Config: &cfg,
		Log:    slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if err := n.Start(ctx); err != nil {
		t.Fatal(err)
	}
	defer n.Shutdown(ctx)

	if !config.Exists(home) {
		t.Fatal("config.json missing after first start")
	}

	client := &http.Client{Timeout: 3 * time.Second}
	statusRes, err := client.Get(n.DashboardURL() + "/api/status")
	if err != nil {
		t.Fatal(err)
	}
	defer statusRes.Body.Close()
	if statusRes.StatusCode != 200 {
		t.Fatal(statusRes.Status)
	}
	var csrf string
	for _, c := range statusRes.Cookies() {
		if c.Name == "fedishare_csrf" {
			csrf = c.Value
		}
	}
	if csrf == "" {
		t.Fatal("missing csrf")
	}

	share := filepath.Join(t.TempDir(), "share")
	payload, err := json.Marshal(map[string]string{
		"display_name":    "Alice",
		"username":        "alice",
		"share_directory": share,
		"gateway_url":     "https://nodes.example.org",
		"summary":         "hi",
	})
	if err != nil {
		t.Fatal(err)
	}
	req, err := http.NewRequest(http.MethodPost, n.DashboardURL()+"/api/setup", bytes.NewReader(payload))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-CSRF-Token", csrf)
	res, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		b, _ := io.ReadAll(res.Body)
		t.Fatalf("setup %d %s", res.StatusCode, b)
	}
	var out map[string]any
	if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	if out["identity"] != "@alice@nodes.example.org" {
		t.Fatalf("identity %#v", out)
	}
	loaded, err := config.Load(home)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Username != "alice" || loaded.ShareDirectory != share {
		t.Fatalf("saved %#v", loaded)
	}
	if _, err := os.Stat(share); err != nil {
		t.Fatal(err)
	}
}

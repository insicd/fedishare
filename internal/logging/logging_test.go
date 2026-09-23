package logging

import (
	"bytes"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseLevel(t *testing.T) {
	if _, err := ParseLevel("verbose"); err == nil {
		t.Fatal("expected error")
	}
	lvl, err := ParseLevel("DEBUG")
	if err != nil || lvl != slog.LevelDebug {
		t.Fatalf("lvl=%v err=%v", lvl, err)
	}
}

func TestRedactPath(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	got := RedactPath(filepath.Join(home, "FediShare", "secret.txt"))
	if !strings.HasPrefix(got, "~") {
		t.Fatalf("expected ~ prefix, got %q", got)
	}
	if strings.Contains(got, home) {
		t.Fatalf("home dir leaked: %q", got)
	}
}

func TestSecretAttributesRedacted(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{
		ReplaceAttr: func(_ []string, a slog.Attr) slog.Attr {
			return redactAttr(a)
		},
	}))
	logger.Info("test", "private_key", "-----BEGIN PRIVATE KEY-----")
	if strings.Contains(buf.String(), "BEGIN PRIVATE KEY") {
		t.Fatalf("private key leaked: %s", buf.String())
	}
	if !strings.Contains(buf.String(), "[redacted]") {
		t.Fatalf("expected redaction, got %s", buf.String())
	}
}

func TestSetupWritesFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "logs", "fedishare.log")
	logger, closeFn, err := Setup("info", path)
	if err != nil {
		t.Fatal(err)
	}
	logger.Info("hello")
	if err := closeFn(); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "hello") {
		t.Fatalf("log file = %q", data)
	}
}

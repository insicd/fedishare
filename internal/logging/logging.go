package logging

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// Setup creates a slog logger that writes to stderr and, when logFile is
// non-empty, to that file. Secrets and private keys must never be passed
// as log attributes.
func Setup(levelName, logFile string) (*slog.Logger, func() error, error) {
	level, err := ParseLevel(levelName)
	if err != nil {
		return nil, nil, err
	}

	writers := []io.Writer{os.Stderr}
	var closer func() error = func() error { return nil }

	if logFile != "" {
		if err := os.MkdirAll(filepath.Dir(logFile), 0o700); err != nil {
			return nil, nil, fmt.Errorf("create log directory: %w", err)
		}
		f, err := os.OpenFile(logFile, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
		if err != nil {
			return nil, nil, fmt.Errorf("open log file: %w", err)
		}
		writers = append(writers, f)
		closer = f.Close
	}

	handler := slog.NewTextHandler(io.MultiWriter(writers...), &slog.HandlerOptions{
		Level: level,
		ReplaceAttr: func(_ []string, a slog.Attr) slog.Attr {
			return redactAttr(a)
		},
	})
	return slog.New(handler), closer, nil
}

func ParseLevel(name string) (slog.Level, error) {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "", "info":
		return slog.LevelInfo, nil
	case "debug":
		return slog.LevelDebug, nil
	case "warn", "warning":
		return slog.LevelWarn, nil
	case "error":
		return slog.LevelError, nil
	default:
		return 0, fmt.Errorf("unsupported log level %q", name)
	}
}

var secretKeys = map[string]struct{}{
	"private_key":     {},
	"privatekey":      {},
	"private_key_pem": {},
	"secret":          {},
	"token":           {},
	"password":        {},
	"authorization":   {},
	"challenge":       {},
	"signature":       {},
}

func redactAttr(a slog.Attr) slog.Attr {
	key := strings.ToLower(a.Key)
	if _, found := secretKeys[key]; found {
		return slog.String(a.Key, "[redacted]")
	}
	if a.Value.Kind() == slog.KindString {
		if key == "path" || key == "share_directory" || key == "file" || key == "dir" {
			return slog.String(a.Key, RedactPath(a.Value.String()))
		}
	}
	return a
}

var (
	homeOnce sync.Once
	homeDir  string
)

// RedactPath replaces the current user's home directory prefix with ~.
func RedactPath(p string) string {
	if p == "" {
		return ""
	}
	homeOnce.Do(func() {
		homeDir, _ = os.UserHomeDir()
	})
	if homeDir != "" && (p == homeDir || strings.HasPrefix(p, homeDir+string(os.PathSeparator))) {
		return "~" + strings.TrimPrefix(p, homeDir)
	}
	return p
}

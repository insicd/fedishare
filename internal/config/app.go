package config

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	appFileName     = "app.json"
	profilesDirName = "profiles"
	profileIDLen    = 8
)

// App is process-wide settings shared by every local profile.
type App struct {
	GatewayURL   string `json:"gateway_url"`
	LocalPort    int    `json:"local_port"`
	LogLevel     string `json:"log_level"`
	StartAtLogin bool   `json:"start_at_login"`
	SelectedID   string `json:"selected_profile"`
}

// ProfileInfo is a dashboard row for one local actor.
type ProfileInfo struct {
	ID             string `json:"id"`
	Username       string `json:"username"`
	DisplayName    string `json:"display_name"`
	ShareDirectory string `json:"share_directory"`
	Identity       string `json:"identity"`
	Selected       bool   `json:"selected"`
	Configured     bool   `json:"configured"`
}

func DefaultApp() App {
	return App{
		GatewayURL: DefaultGatewayURL,
		LocalPort:  DefaultLocalPort,
		LogLevel:   DefaultLogLevel,
	}
}

func AppPath(home string) string {
	return filepath.Join(home, appFileName)
}

func AppExists(home string) bool {
	_, err := os.Stat(AppPath(home))
	return err == nil
}

func ProfilesDir(home string) string {
	return filepath.Join(home, profilesDirName)
}

func ProfileHome(home, id string) string {
	return filepath.Join(ProfilesDir(home), id)
}

func (a *App) Normalize() {
	a.GatewayURL = strings.TrimSpace(a.GatewayURL)
	a.LogLevel = strings.ToLower(strings.TrimSpace(a.LogLevel))
	a.SelectedID = strings.ToLower(strings.TrimSpace(a.SelectedID))
	if a.LocalPort < 0 {
		a.LocalPort = DefaultLocalPort
	}
	if a.LogLevel == "" {
		a.LogLevel = DefaultLogLevel
	}
}

func LoadApp(home string) (*App, error) {
	data, err := os.ReadFile(AppPath(home))
	if errors.Is(err, os.ErrNotExist) {
		app := DefaultApp()
		return &app, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read app settings: %w", err)
	}
	app := DefaultApp()
	if err := json.Unmarshal(data, &app); err != nil {
		return nil, fmt.Errorf("parse app settings: %w", err)
	}
	app.Normalize()
	return &app, nil
}

func (a App) Save(home string) error {
	a.Normalize()
	if err := EnsureHome(home); err != nil {
		return err
	}
	if err := os.MkdirAll(ProfilesDir(home), 0o700); err != nil {
		return fmt.Errorf("create profiles dir: %w", err)
	}
	data, err := json.MarshalIndent(a, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	path := AppPath(home)
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return fmt.Errorf("write app settings: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return os.Chmod(path, 0o600)
}

// NewProfileID returns a random opaque profile directory name.
func NewProfileID() (string, error) {
	b := make([]byte, profileIDLen)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// ListProfileIDs returns profile directory names under home/profiles.
func ListProfileIDs(home string) ([]string, error) {
	entries, err := os.ReadDir(ProfilesDir(home))
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var ids []string
	for _, e := range entries {
		if e.IsDir() && !strings.HasPrefix(e.Name(), ".") {
			ids = append(ids, e.Name())
		}
	}
	return ids, nil
}

// ApplyAppDefaults copies process-wide fields onto a profile config.
func ApplyAppDefaults(cfg *Config, app App) {
	if cfg == nil {
		return
	}
	cfg.GatewayURL = app.GatewayURL
	cfg.LocalPort = app.LocalPort
	cfg.LogLevel = app.LogLevel
	cfg.StartAtLogin = app.StartAtLogin
	cfg.Normalize()
}

// MigrateLegacyHome moves a Phase-6 single-actor home into profiles/.
func MigrateLegacyHome(home string) error {
	if AppExists(home) {
		return nil
	}
	if err := os.MkdirAll(ProfilesDir(home), 0o700); err != nil {
		return err
	}
	app := DefaultApp()
	if !Exists(home) {
		return app.Save(home)
	}
	cfg, err := Load(home)
	if err != nil {
		return err
	}
	app.GatewayURL = cfg.GatewayURL
	app.LocalPort = cfg.LocalPort
	app.LogLevel = cfg.LogLevel
	app.StartAtLogin = cfg.StartAtLogin
	if !cfg.Configured() {
		_ = os.Remove(Path(home))
		return app.Save(home)
	}
	id, err := NewProfileID()
	if err != nil {
		return err
	}
	dest := ProfileHome(home, id)
	if err := os.MkdirAll(dest, 0o700); err != nil {
		return err
	}
	for _, name := range []string{fileName, "fedishare.db", "fedishare.db-wal", "fedishare.db-shm", "keys"} {
		src := filepath.Join(home, name)
		if _, err := os.Stat(src); err != nil {
			continue
		}
		if err := os.Rename(src, filepath.Join(dest, name)); err != nil {
			return fmt.Errorf("migrate %s: %w", name, err)
		}
	}
	app.SelectedID = id
	return app.Save(home)
}

package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"unicode/utf8"
)

const (
	fileName = "config.json"

	DefaultLocalPort              = 17890
	DefaultMaxConcurrentDownloads = 8
	DefaultLogLevel               = "info"

	maxDisplayName = 80
	maxSummary     = 500
	maxUsername    = 30
)

var (
	usernameRE = regexp.MustCompile(`^[a-z0-9_]{1,30}$`)

	// ErrNotConfigured means the user has not completed first-run setup.
	ErrNotConfigured = errors.New("fedishare is not configured")
)

// Config is the user-facing settings file stored in the OS config directory.
// It is separate from the shared folder and from SQLite runtime state.
type Config struct {
	Username               string `json:"username"`
	DisplayName            string `json:"display_name"`
	Summary                string `json:"summary"`
	ShareDirectory         string `json:"share_directory"`
	GatewayURL             string `json:"gateway_url"`
	LocalPort              int    `json:"local_port"`
	MaxConcurrentDownloads int    `json:"max_concurrent_downloads"`
	BandwidthLimitBPS      int64  `json:"bandwidth_limit_bps"`
	LogLevel               string `json:"log_level"`
	StartAtLogin           bool   `json:"start_at_login"`
}

func Default() Config {
	return Config{
		LocalPort:              DefaultLocalPort,
		MaxConcurrentDownloads: DefaultMaxConcurrentDownloads,
		BandwidthLimitBPS:      0,
		LogLevel:               DefaultLogLevel,
		StartAtLogin:           false,
	}
}

// Configured reports whether the minimum first-run fields are present.
func (c Config) Configured() bool {
	return c.Username != "" && c.ShareDirectory != ""
}

func (c *Config) Normalize() {
	c.Username = strings.ToLower(strings.TrimSpace(c.Username))
	c.DisplayName = strings.TrimSpace(c.DisplayName)
	c.Summary = strings.TrimSpace(c.Summary)
	c.ShareDirectory = strings.TrimSpace(c.ShareDirectory)
	c.GatewayURL = strings.TrimSpace(c.GatewayURL)
	c.LogLevel = strings.ToLower(strings.TrimSpace(c.LogLevel))
	if c.MaxConcurrentDownloads == 0 {
		c.MaxConcurrentDownloads = DefaultMaxConcurrentDownloads
	}
	if c.LogLevel == "" {
		c.LogLevel = DefaultLogLevel
	}
}

func (c Config) Validate() error {
	c.Normalize()
	if c.Username != "" && !usernameRE.MatchString(c.Username) {
		return fmt.Errorf("username must be 1–%d characters of lowercase letters, digits, or underscore", maxUsername)
	}
	if c.DisplayName != "" {
		if utf8.RuneCountInString(c.DisplayName) > maxDisplayName {
			return fmt.Errorf("display name must be at most %d characters", maxDisplayName)
		}
		if containsCtl(c.DisplayName) {
			return errors.New("display name contains invalid characters")
		}
	}
	if utf8.RuneCountInString(c.Summary) > maxSummary {
		return fmt.Errorf("bio must be at most %d characters", maxSummary)
	}
	if c.ShareDirectory != "" && !filepath.IsAbs(c.ShareDirectory) {
		return errors.New("shared directory must be an absolute path")
	}
	if c.GatewayURL != "" {
		u, err := url.Parse(c.GatewayURL)
		if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
			return errors.New("gateway URL must be an http or https URL")
		}
	}
	if c.LocalPort < 0 || c.LocalPort > 65535 {
		return errors.New("local port is out of range")
	}
	if c.MaxConcurrentDownloads < 1 {
		return errors.New("max concurrent downloads must be at least 1")
	}
	if c.BandwidthLimitBPS < 0 {
		return errors.New("bandwidth limit cannot be negative")
	}
	switch c.LogLevel {
	case "debug", "info", "warn", "error":
	default:
		return fmt.Errorf("unsupported log level %q", c.LogLevel)
	}
	return nil
}

func containsCtl(s string) bool {
	for _, r := range s {
		if r < 32 || r == 127 {
			return true
		}
	}
	return false
}

// DefaultHomeDir is the OS-specific application data directory.
// Configuration and the SQLite database live here, never in the shared folder.
func DefaultHomeDir() (string, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("resolve user config dir: %w", err)
	}
	return filepath.Join(base, appDirName()), nil
}

func appDirName() string {
	switch runtime.GOOS {
	case "windows", "darwin":
		return "FediShare"
	default:
		return "fedishare"
	}
}

// ResolveHome returns override (made absolute) or the OS default home.
func ResolveHome(override string) (string, error) {
	if strings.TrimSpace(override) == "" {
		return DefaultHomeDir()
	}
	abs, err := filepath.Abs(override)
	if err != nil {
		return "", fmt.Errorf("resolve data dir: %w", err)
	}
	return abs, nil
}

func Path(home string) string {
	return filepath.Join(home, fileName)
}

// Exists reports whether config.json is already on disk.
func Exists(home string) bool {
	_, err := os.Stat(Path(home))
	return err == nil
}

// LoadOrCreate loads config.json, or writes defaults on first run.
// The boolean is true when a new file was created.
func LoadOrCreate(home string) (*Config, bool, error) {
	if Exists(home) {
		cfg, err := Load(home)
		return cfg, false, err
	}
	cfg := Default()
	if err := cfg.Save(home); err != nil {
		return nil, false, err
	}
	return &cfg, true, nil
}

// DefaultShareDirectory is the suggested folder shown in the first-run wizard.
func DefaultShareDirectory() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, "FediShare")
}

// FediverseAddress returns @user@gateway-host, or @user when the gateway is unset.
func (c Config) FediverseAddress() string {
	if c.Username == "" {
		return ""
	}
	host := GatewayHost(c.GatewayURL)
	if host == "" {
		return "@" + c.Username
	}
	return "@" + c.Username + "@" + host
}

// PublicBase is the origin used in ActivityPub ids.
// When a gateway URL is set that is the public identity; otherwise fallback
// (usually the loopback dashboard) is used so local discovery still works.
func PublicBase(gatewayURL, fallback string) string {
	base := strings.TrimRight(strings.TrimSpace(gatewayURL), "/")
	if base != "" {
		return base
	}
	return strings.TrimRight(strings.TrimSpace(fallback), "/")
}

func (c Config) PublicBase(fallback string) string {
	return PublicBase(c.GatewayURL, fallback)
}

// AcctHost is the host part of acct:user@host.
func AcctHost(gatewayURL, fallback string) string {
	if h := GatewayHost(gatewayURL); h != "" {
		return h
	}
	if h := GatewayHost(fallback); h != "" {
		return h
	}
	return strings.TrimSpace(fallback)
}

func (c Config) AcctHost(fallback string) string {
	return AcctHost(c.GatewayURL, fallback)
}

// GatewayHost extracts the hostname from a gateway URL.
func GatewayHost(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	u, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	return u.Hostname()
}

// ValidateShareRoot rejects a share folder that would expose FediShare's own data.
func ValidateShareRoot(shareDir, dataHome string) error {
	if shareDir == "" {
		return errors.New("shared directory is required")
	}
	if !filepath.IsAbs(shareDir) {
		return errors.New("shared directory must be an absolute path")
	}
	shareDir = filepath.Clean(shareDir)
	dataHome = filepath.Clean(dataHome)
	if shareDir == dataHome {
		return errors.New("shared directory cannot be FediShare's data directory")
	}
	if containsPath(shareDir, dataHome) {
		return errors.New("shared directory cannot be inside FediShare's data directory")
	}
	if containsPath(dataHome, shareDir) {
		return errors.New("shared directory cannot contain FediShare's data directory")
	}
	return nil
}

func containsPath(child, parent string) bool {
	rel, err := filepath.Rel(parent, child)
	if err != nil {
		return false
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(os.PathSeparator))
}

func DatabasePath(home string) string {
	return filepath.Join(home, "fedishare.db")
}

func LogPath(home string) string {
	return filepath.Join(home, "logs", "fedishare.log")
}

func KeysDir(home string) string {
	return filepath.Join(home, "keys")
}

// EnsureHome creates the data directory layout with restrictive permissions.
func EnsureHome(home string) error {
	if err := os.MkdirAll(home, 0o700); err != nil {
		return fmt.Errorf("create data dir: %w", err)
	}
	if err := os.MkdirAll(filepath.Join(home, "logs"), 0o700); err != nil {
		return fmt.Errorf("create log dir: %w", err)
	}
	if err := os.MkdirAll(KeysDir(home), 0o700); err != nil {
		return fmt.Errorf("create keys dir: %w", err)
	}
	return nil
}

// Load reads config.json. A missing file returns defaults and is not an error.
func Load(home string) (*Config, error) {
	path := Path(home)
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		cfg := Default()
		return &cfg, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}
	cfg := Default()
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}
	cfg.Normalize()
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	return &cfg, nil
}

// Save writes config.json atomically with 0600 permissions.
func (c Config) Save(home string) error {
	c.Normalize()
	if err := c.Validate(); err != nil {
		return err
	}
	if err := EnsureHome(home); err != nil {
		return err
	}
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return fmt.Errorf("encode config: %w", err)
	}
	data = append(data, '\n')
	path := Path(home)
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return fmt.Errorf("write config: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("replace config: %w", err)
	}
	if err := os.Chmod(path, 0o600); err != nil {
		return fmt.Errorf("restrict config permissions: %w", err)
	}
	return nil
}

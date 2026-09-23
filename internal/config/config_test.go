package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestDefaultAndConfigured(t *testing.T) {
	cfg := Default()
	if cfg.Configured() {
		t.Fatal("empty default config should not be configured")
	}
	if cfg.LocalPort != DefaultLocalPort {
		t.Fatalf("port = %d", cfg.LocalPort)
	}
	cfg.Username = "alice"
	cfg.ShareDirectory = "/tmp/share"
	if !cfg.Configured() {
		t.Fatal("expected configured")
	}
}

func TestValidateUsername(t *testing.T) {
	cfg := Default()
	cfg.Username = "Alice"
	cfg.Normalize()
	if cfg.Username != "alice" {
		t.Fatalf("username not normalized: %q", cfg.Username)
	}
	cfg.Username = "alice-bob"
	if err := cfg.Validate(); err == nil {
		t.Fatal("hyphen should be rejected")
	}
	cfg.Username = "alice"
	if err := cfg.Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestValidateShareDirectoryMustBeAbsolute(t *testing.T) {
	cfg := Default()
	cfg.ShareDirectory = "relative/path"
	if err := cfg.Validate(); err == nil {
		t.Fatal("relative share directory should be rejected")
	}
}

func TestPublicBaseAndAcctHost(t *testing.T) {
	if PublicBase("https://nodes.example.org/", "http://127.0.0.1:1") != "https://nodes.example.org" {
		t.Fatal("gateway should win")
	}
	if PublicBase("", "http://127.0.0.1:17890") != "http://127.0.0.1:17890" {
		t.Fatal("fallback")
	}
	if AcctHost("https://nodes.example.org:443", "http://127.0.0.1:17890") != "nodes.example.org" {
		t.Fatalf("acct host=%s", AcctHost("https://nodes.example.org:443", "http://127.0.0.1:17890"))
	}
	if AcctHost("", "http://127.0.0.1:17890") != "127.0.0.1" {
		t.Fatalf("loopback acct=%s", AcctHost("", "http://127.0.0.1:17890"))
	}
}

func TestValidateGatewayURL(t *testing.T) {
	cfg := Default()
	cfg.GatewayURL = "ftp://example.com"
	if err := cfg.Validate(); err == nil {
		t.Fatal("ftp should be rejected")
	}
	cfg.GatewayURL = "https://nodes.example.org"
	if err := cfg.Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestSaveLoadRoundTrip(t *testing.T) {
	home := t.TempDir()
	cfg := Default()
	cfg.Username = "alice"
	cfg.DisplayName = "Alice"
	cfg.Summary = "Sharing files."
	cfg.ShareDirectory = filepath.Join(home, "share")
	cfg.GatewayURL = "https://nodes.example.org"
	if err := cfg.Save(home); err != nil {
		t.Fatal(err)
	}

	info, err := os.Stat(Path(home))
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm() != 0o600 {
		t.Fatalf("config permissions = %o", info.Mode().Perm())
	}

	loaded, err := Load(home)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Username != "alice" || loaded.DisplayName != "Alice" {
		t.Fatalf("loaded %#v", loaded)
	}
	if loaded.ShareDirectory != cfg.ShareDirectory {
		t.Fatalf("share dir %q", loaded.ShareDirectory)
	}
}

func TestLoadMissingReturnsDefaults(t *testing.T) {
	cfg, err := Load(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Configured() {
		t.Fatal("missing file must not look configured")
	}
}

func TestLoadOrCreateWritesFile(t *testing.T) {
	home := t.TempDir()
	if Exists(home) {
		t.Fatal("file should not exist yet")
	}
	cfg, created, err := LoadOrCreate(home)
	if err != nil {
		t.Fatal(err)
	}
	if !created {
		t.Fatal("expected a new file")
	}
	if !Exists(home) {
		t.Fatal("config.json was not created")
	}
	if cfg.Configured() {
		t.Fatal("defaults must remain unconfigured")
	}
	if cfg.LocalPort != DefaultLocalPort {
		t.Fatalf("port = %d", cfg.LocalPort)
	}
	again, created, err := LoadOrCreate(home)
	if err != nil || created {
		t.Fatalf("second load created=%v err=%v", created, err)
	}
	if again.LocalPort != DefaultLocalPort {
		t.Fatalf("reloaded port = %d", again.LocalPort)
	}
}

func TestFediverseAddress(t *testing.T) {
	cfg := Default()
	if cfg.FediverseAddress() != "" {
		t.Fatal("empty username")
	}
	cfg.Username = "alice"
	if cfg.FediverseAddress() != "@alice" {
		t.Fatalf("got %q", cfg.FediverseAddress())
	}
	cfg.GatewayURL = "https://nodes.example.org"
	if cfg.FediverseAddress() != "@alice@nodes.example.org" {
		t.Fatalf("got %q", cfg.FediverseAddress())
	}
}

func TestValidateShareRoot(t *testing.T) {
	home := t.TempDir()
	share := filepath.Join(t.TempDir(), "share")
	if err := ValidateShareRoot(share, home); err != nil {
		t.Fatal(err)
	}
	if err := ValidateShareRoot(home, home); err == nil {
		t.Fatal("data dir as share")
	}
	if err := ValidateShareRoot(filepath.Join(home, "keys"), home); err == nil {
		t.Fatal("inside data dir")
	}
	parent := filepath.Dir(home)
	if err := ValidateShareRoot(parent, home); err == nil {
		t.Fatal("parent of data dir")
	}
}

func TestLoadInvalidJSON(t *testing.T) {
	home := t.TempDir()
	if err := os.WriteFile(Path(home), []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(home); err == nil {
		t.Fatal("expected parse error")
	}
}

func TestSaveAtomicJSON(t *testing.T) {
	home := t.TempDir()
	cfg := Default()
	cfg.Username = "bob"
	cfg.ShareDirectory = filepath.Join(home, "s")
	if err := cfg.Save(home); err != nil {
		t.Fatal(err)
	}
	var parsed Config
	data, err := os.ReadFile(Path(home))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatal(err)
	}
}

func TestAppDirName(t *testing.T) {
	name := appDirName()
	switch runtime.GOOS {
	case "windows", "darwin":
		if name != "FediShare" {
			t.Fatalf("got %s", name)
		}
	default:
		if name != "fedishare" {
			t.Fatalf("got %s", name)
		}
	}
}

func TestResolveHomeOverride(t *testing.T) {
	dir := t.TempDir()
	got, err := ResolveHome(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got != dir {
		t.Fatalf("got %s want %s", got, dir)
	}
}

func TestEnsureHomeCreatesKeysDir(t *testing.T) {
	home := t.TempDir()
	sub := filepath.Join(home, "nested")
	if err := EnsureHome(sub); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(KeysDir(sub))
	if err != nil {
		t.Fatal(err)
	}
	if !info.IsDir() {
		t.Fatal("keys dir missing")
	}
}

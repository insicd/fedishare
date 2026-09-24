package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestMigrateLegacyHomeMovesConfiguredNode(t *testing.T) {
	home := t.TempDir()
	share := filepath.Join(t.TempDir(), "share")
	if err := os.MkdirAll(share, 0o755); err != nil {
		t.Fatal(err)
	}
	cfg := Default()
	cfg.Username = "alice"
	cfg.ShareDirectory = share
	cfg.GatewayURL = ""
	if err := cfg.Save(home); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, "fedishare.db"), []byte("db"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(home, "keys"), 0o700); err != nil && !os.IsExist(err) {
		t.Fatal(err)
	}
	if err := MigrateLegacyHome(home); err != nil {
		t.Fatal(err)
	}
	if Exists(home) {
		t.Fatal("legacy config.json should have moved")
	}
	if !AppExists(home) {
		t.Fatal("app.json missing")
	}
	ids, err := ListProfileIDs(home)
	if err != nil || len(ids) != 1 {
		t.Fatalf("ids=%v err=%v", ids, err)
	}
	loaded, err := Load(ProfileHome(home, ids[0]))
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Username != "alice" {
		t.Fatalf("%+v", loaded)
	}
}

func TestMigrateLegacyHomeEmptyConfig(t *testing.T) {
	home := t.TempDir()
	cfg := Default()
	cfg.GatewayURL = ""
	if err := cfg.Save(home); err != nil {
		t.Fatal(err)
	}
	if err := MigrateLegacyHome(home); err != nil {
		t.Fatal(err)
	}
	ids, err := ListProfileIDs(home)
	if err != nil || len(ids) != 0 {
		t.Fatalf("ids=%v err=%v", ids, err)
	}
	if Exists(home) {
		t.Fatal("unconfigured root config should be removed")
	}
}

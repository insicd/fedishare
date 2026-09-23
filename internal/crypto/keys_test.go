package crypto

import (
	"crypto/x509"
	"encoding/pem"
	"os"
	"runtime"
	"strings"
	"testing"
)

func TestLoadOrCreatePersistsAndDoesNotRotate(t *testing.T) {
	dir := t.TempDir()
	store := KeyStore{Dir: dir}
	if err := store.LoadOrCreate(); err != nil {
		t.Fatal(err)
	}
	first, err := store.PublicPEM()
	if err != nil {
		t.Fatal(err)
	}
	if err := store.LoadOrCreate(); err != nil {
		t.Fatal(err)
	}
	second, err := store.PublicPEM()
	if err != nil {
		t.Fatal(err)
	}
	if string(first) != string(second) {
		t.Fatal("key pair was rotated")
	}

	info, err := os.Stat(store.PrivatePath())
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm() != 0o600 {
		t.Fatalf("private key permissions = %o", info.Mode().Perm())
	}

	block, _ := pem.Decode(first)
	if block == nil {
		t.Fatal("public pem")
	}
	if _, err := x509.ParsePKIXPublicKey(block.Bytes); err != nil {
		t.Fatal(err)
	}
}

func TestPrivateAndPublicKeyRoundTrip(t *testing.T) {
	store := KeyStore{Dir: t.TempDir()}
	if err := store.LoadOrCreate(); err != nil {
		t.Fatal(err)
	}
	priv, err := store.PrivateKey()
	if err != nil {
		t.Fatal(err)
	}
	pub, err := store.PublicKey()
	if err != nil {
		t.Fatal(err)
	}
	if priv.N.Cmp(pub.N) != 0 {
		t.Fatal("public key does not match private key")
	}
	pem, err := store.PublicKeyPEM()
	if err != nil || !strings.Contains(pem, "BEGIN PUBLIC KEY") {
		t.Fatalf("pem=%q err=%v", pem, err)
	}
}

func TestPublicPEMRejectsGarbage(t *testing.T) {
	dir := t.TempDir()
	store := KeyStore{Dir: dir}
	if err := os.WriteFile(store.PublicPath(), []byte("not-a-key"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := store.PublicPEM(); err == nil {
		t.Fatal("expected error")
	}
}

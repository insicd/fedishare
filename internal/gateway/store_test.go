package gateway

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"
)

func TestRegisterFirstKeyWins(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "gateway.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	ctx := context.Background()
	if err := store.Register(ctx, "Alice", "PEM-A", "n1"); err != nil {
		t.Fatal(err)
	}
	if err := store.Register(ctx, "alice", "PEM-A", "n2"); err != nil {
		t.Fatal(err)
	}
	if err := store.Register(ctx, "alice", "PEM-B", "n3"); !errors.Is(err, ErrUsernameTaken) {
		t.Fatalf("err = %v", err)
	}
	row, err := store.Get(ctx, "alice")
	if err != nil {
		t.Fatal(err)
	}
	if row.PublicKeyPEM != "PEM-A" || row.NodeID != "n2" {
		t.Fatalf("%+v", row)
	}
}

func TestNonceReplay(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "gateway.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	ctx := context.Background()
	exp := time.Now().Add(10 * time.Minute)
	if err := store.ConsumeNonce(ctx, "abc", exp); err != nil {
		t.Fatal(err)
	}
	if err := store.ConsumeNonce(ctx, "abc", exp); err == nil {
		t.Fatal("expected replay")
	}
}

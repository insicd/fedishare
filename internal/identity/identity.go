package identity

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"

	"github.com/fedishare/fedishare/internal/database"
)

const (
	// ConfigKeyNodeID is the SQLite config key for the installation ID.
	// This is not the ActivityPub identity. Actor keys live in the keys
	// directory and never in this value.
	ConfigKeyNodeID       = "node_id"
	ConfigKeyFirstStarted = "first_started_at"
	ConfigKeyLastStarted  = "last_started_at"
)

// LoadOrCreateNodeID returns a stable per-installation identifier.
func LoadOrCreateNodeID(ctx context.Context, db *database.DB) (string, error) {
	existing, ok, err := db.GetConfig(ctx, ConfigKeyNodeID)
	if err != nil {
		return "", err
	}
	if ok && existing != "" {
		return existing, nil
	}
	id, err := newNodeID()
	if err != nil {
		return "", err
	}
	if err := db.SetConfig(ctx, ConfigKeyNodeID, id); err != nil {
		return "", err
	}
	return id, nil
}

func newNodeID() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", fmt.Errorf("generate node id: %w", err)
	}
	return hex.EncodeToString(b[:]), nil
}

// RecordStart timestamps this process start in the config table.
func RecordStart(ctx context.Context, db *database.DB, startedAt string) error {
	if _, ok, err := db.GetConfig(ctx, ConfigKeyFirstStarted); err != nil {
		return err
	} else if !ok {
		if err := db.SetConfig(ctx, ConfigKeyFirstStarted, startedAt); err != nil {
			return err
		}
	}
	return db.SetConfig(ctx, ConfigKeyLastStarted, startedAt)
}

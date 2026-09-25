package gateway

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

// ErrUsernameTaken means another public key already owns this username.
var ErrUsernameTaken = errors.New("username is already registered to a different key")

// ActorRow is the public metadata the gateway is allowed to persist.
// It never includes a private key or file bytes.
type ActorRow struct {
	Username      string
	PublicKeyPEM  string
	NodeID        string
	ActorJSON     string
	WebFingerJSON string
}

type Store struct {
	sql  *sql.DB
	path string
}

func Open(path string) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, fmt.Errorf("create gateway data directory: %w", err)
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open gateway sqlite: %w", err)
	}
	db.SetMaxOpenConns(4)
	db.SetConnMaxLifetime(0)
	s := &Store{sql: db, path: path}
	for _, p := range []string{
		"PRAGMA foreign_keys = ON",
		"PRAGMA journal_mode = WAL",
		"PRAGMA busy_timeout = 5000",
		"PRAGMA synchronous = NORMAL",
	} {
		if _, err := db.Exec(p); err != nil {
			_ = db.Close()
			return nil, fmt.Errorf("%s: %w", p, err)
		}
	}
	if err := s.migrate(); err != nil {
		_ = db.Close()
		return nil, err
	}
	_ = os.Chmod(path, 0o600)
	return s, nil
}

func (s *Store) migrate() error {
	_, err := s.sql.Exec(`
CREATE TABLE IF NOT EXISTS actors (
	username TEXT PRIMARY KEY,
	public_key_pem TEXT NOT NULL,
	node_id TEXT,
	actor_json TEXT,
	webfinger_json TEXT,
	first_seen TEXT NOT NULL,
	last_seen TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS used_nonces (
	nonce TEXT PRIMARY KEY,
	expires_at TEXT NOT NULL
);`)
	return err
}

func (s *Store) Close() error {
	if s == nil || s.sql == nil {
		return nil
	}
	return s.sql.Close()
}

func normalizePEM(pem string) string {
	return strings.TrimSpace(pem)
}

// Register binds username to publicKeyPEM. The first key wins.
func (s *Store) Register(ctx context.Context, username, publicKeyPEM, nodeID string) error {
	username = strings.ToLower(strings.TrimSpace(username))
	publicKeyPEM = normalizePEM(publicKeyPEM)
	if username == "" || publicKeyPEM == "" {
		return fmt.Errorf("username and public key are required")
	}
	now := time.Now().UTC().Format(time.RFC3339)
	var existing string
	err := s.sql.QueryRowContext(ctx, `SELECT public_key_pem FROM actors WHERE username = ?`, username).Scan(&existing)
	if err == sql.ErrNoRows {
		_, err = s.sql.ExecContext(ctx, `
			INSERT INTO actors (username, public_key_pem, node_id, first_seen, last_seen)
			VALUES (?, ?, ?, ?, ?)`, username, publicKeyPEM, nodeID, now, now)
		return err
	}
	if err != nil {
		return err
	}
	if normalizePEM(existing) != publicKeyPEM {
		return ErrUsernameTaken
	}
	_, err = s.sql.ExecContext(ctx, `
		UPDATE actors SET node_id = ?, last_seen = ? WHERE username = ?`, nodeID, now, username)
	return err
}

func (s *Store) List(ctx context.Context, limit int) ([]ActorRow, error) {
	if limit <= 0 || limit > 500 {
		limit = 500
	}
	rows, err := s.sql.QueryContext(ctx, `
		SELECT username, public_key_pem, COALESCE(node_id, ''), COALESCE(actor_json, ''), COALESCE(webfinger_json, '')
		FROM actors
		ORDER BY username
		LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ActorRow
	for rows.Next() {
		var row ActorRow
		if err := rows.Scan(&row.Username, &row.PublicKeyPEM, &row.NodeID, &row.ActorJSON, &row.WebFingerJSON); err != nil {
			return nil, err
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

func (s *Store) Get(ctx context.Context, username string) (ActorRow, error) {
	username = strings.ToLower(strings.TrimSpace(username))
	var row ActorRow
	err := s.sql.QueryRowContext(ctx, `
		SELECT username, public_key_pem, COALESCE(node_id, ''), COALESCE(actor_json, ''), COALESCE(webfinger_json, '')
		FROM actors WHERE username = ?`, username).Scan(
		&row.Username, &row.PublicKeyPEM, &row.NodeID, &row.ActorJSON, &row.WebFingerJSON)
	if err == sql.ErrNoRows {
		return ActorRow{}, err
	}
	return row, err
}

func (s *Store) SaveActor(ctx context.Context, username, body string) error {
	username = strings.ToLower(strings.TrimSpace(username))
	_, err := s.sql.ExecContext(ctx, `UPDATE actors SET actor_json = ? WHERE username = ?`, body, username)
	return err
}

func (s *Store) SaveWebFinger(ctx context.Context, username, body string) error {
	username = strings.ToLower(strings.TrimSpace(username))
	_, err := s.sql.ExecContext(ctx, `UPDATE actors SET webfinger_json = ? WHERE username = ?`, body, username)
	return err
}

func (s *Store) ConsumeNonce(ctx context.Context, nonce string, expires time.Time) error {
	if nonce == "" {
		return fmt.Errorf("empty nonce")
	}
	_, err := s.sql.ExecContext(ctx, `DELETE FROM used_nonces WHERE expires_at < ?`, time.Now().UTC().Format(time.RFC3339))
	if err != nil {
		return err
	}
	_, err = s.sql.ExecContext(ctx, `INSERT INTO used_nonces (nonce, expires_at) VALUES (?, ?)`,
		nonce, expires.UTC().Format(time.RFC3339))
	if err != nil {
		return fmt.Errorf("nonce already used")
	}
	return nil
}

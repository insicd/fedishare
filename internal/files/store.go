package files

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/fedishare/fedishare/internal/database"
)

type Store struct {
	db *database.DB
}

func NewStore(db *database.DB) *Store { return &Store{db: db} }

func newOpaqueID() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(b[:]), nil
}

func (s *Store) GetByID(ctx context.Context, id string) (Record, error) {
	return s.scanOne(ctx, `SELECT id, relative_path, filename, mime_type, size, hash_algorithm, hash_digest, modified_at, indexed_at, available, visibility FROM files WHERE id = ?`, id)
}

func (s *Store) GetByPath(ctx context.Context, rel string) (Record, error) {
	return s.scanOne(ctx, `SELECT id, relative_path, filename, mime_type, size, hash_algorithm, hash_digest, modified_at, indexed_at, available, visibility FROM files WHERE relative_path = ?`, rel)
}

func (s *Store) ListByHash(ctx context.Context, algo, digest string) ([]Record, error) {
	rows, err := s.db.SQL().QueryContext(ctx, `
		SELECT id, relative_path, filename, mime_type, size, hash_algorithm, hash_digest, modified_at, indexed_at, available, visibility
		FROM files WHERE hash_algorithm = ? AND hash_digest = ?`, algo, digest)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanRows(rows)
}

func (s *Store) DeleteByID(ctx context.Context, id string) error {
	_, err := s.db.SQL().ExecContext(ctx, `DELETE FROM files WHERE id = ?`, id)
	return err
}

// Save writes rec by id when rec.ID is set, otherwise by path.
func (s *Store) Save(ctx context.Context, rec Record) (Record, error) {
	if rec.ID == "" {
		return s.Upsert(ctx, rec)
	}
	if _, err := s.GetByID(ctx, rec.ID); err != nil {
		return s.Upsert(ctx, rec)
	}
	if other, err := s.GetByPath(ctx, rec.RelativePath); err == nil && other.ID != rec.ID {
		if err := s.DeleteByID(ctx, other.ID); err != nil {
			return Record{}, err
		}
	}
	if rec.Visibility == "" {
		rec.Visibility = VisibilityPublic
	}
	now := time.Now().UTC()
	if rec.IndexedAt.IsZero() {
		rec.IndexedAt = now
	}
	avail := 0
	if rec.Available {
		avail = 1
	}
	_, err := s.db.SQL().ExecContext(ctx, `
		UPDATE files SET
			relative_path = ?, filename = ?, mime_type = ?, size = ?,
			hash_algorithm = ?, hash_digest = ?, modified_at = ?, indexed_at = ?,
			available = ?, visibility = ?
		WHERE id = ?`,
		rec.RelativePath, rec.Filename, rec.MIMEType, rec.Size,
		rec.Hash.Algorithm, rec.Hash.Digest,
		rec.ModifiedAt.UTC().Format(time.RFC3339),
		rec.IndexedAt.UTC().Format(time.RFC3339),
		avail, rec.Visibility, rec.ID)
	if err != nil {
		return Record{}, fmt.Errorf("save file: %w", err)
	}
	return rec, nil
}

func (s *Store) scanOne(ctx context.Context, q string, arg any) (Record, error) {
	var rec Record
	var modified, indexed string
	var available int
	err := s.db.SQL().QueryRowContext(ctx, q, arg).Scan(
		&rec.ID, &rec.RelativePath, &rec.Filename, &rec.MIMEType, &rec.Size,
		&rec.Hash.Algorithm, &rec.Hash.Digest, &modified, &indexed, &available, &rec.Visibility,
	)
	if err == sql.ErrNoRows {
		return Record{}, ErrUnavailable
	}
	if err != nil {
		return Record{}, err
	}
	rec.Available = available == 1
	rec.ModifiedAt, _ = time.Parse(time.RFC3339, modified)
	rec.IndexedAt, _ = time.Parse(time.RFC3339, indexed)
	return rec, nil
}

func (s *Store) Upsert(ctx context.Context, rec Record) (Record, error) {
	if rec.ID == "" {
		existing, err := s.GetByPath(ctx, rec.RelativePath)
		if err == nil {
			rec.ID = existing.ID
		} else {
			id, err := newOpaqueID()
			if err != nil {
				return Record{}, err
			}
			rec.ID = id
		}
	}
	if rec.Visibility == "" {
		rec.Visibility = VisibilityPublic
	}
	now := time.Now().UTC()
	if rec.IndexedAt.IsZero() {
		rec.IndexedAt = now
	}
	avail := 0
	if rec.Available {
		avail = 1
	}
	_, err := s.db.SQL().ExecContext(ctx, `
		INSERT INTO files (id, relative_path, filename, mime_type, size, hash_algorithm, hash_digest, modified_at, indexed_at, available, visibility)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(relative_path) DO UPDATE SET
			filename = excluded.filename,
			mime_type = excluded.mime_type,
			size = excluded.size,
			hash_algorithm = excluded.hash_algorithm,
			hash_digest = excluded.hash_digest,
			modified_at = excluded.modified_at,
			indexed_at = excluded.indexed_at,
			available = excluded.available,
			visibility = excluded.visibility`,
		rec.ID, rec.RelativePath, rec.Filename, rec.MIMEType, rec.Size,
		rec.Hash.Algorithm, rec.Hash.Digest,
		rec.ModifiedAt.UTC().Format(time.RFC3339),
		rec.IndexedAt.UTC().Format(time.RFC3339),
		avail, rec.Visibility,
	)
	if err != nil {
		return Record{}, fmt.Errorf("upsert file: %w", err)
	}
	return rec, nil
}

func (s *Store) MarkMissing(ctx context.Context, rel string) error {
	_, err := s.db.SQL().ExecContext(ctx, `UPDATE files SET available = 0, indexed_at = ? WHERE relative_path = ?`,
		time.Now().UTC().Format(time.RFC3339), rel)
	return err
}

func (s *Store) MarkAllMissing(ctx context.Context) error {
	_, err := s.db.SQL().ExecContext(ctx, `UPDATE files SET available = 0, indexed_at = ?`,
		time.Now().UTC().Format(time.RFC3339))
	return err
}

func (s *Store) ListAvailable(ctx context.Context, limit int) ([]Record, error) {
	if limit <= 0 {
		limit = 200
	}
	rows, err := s.db.SQL().QueryContext(ctx, `
		SELECT id, relative_path, filename, mime_type, size, hash_algorithm, hash_digest, modified_at, indexed_at, available, visibility
		FROM files WHERE available = 1 ORDER BY filename COLLATE NOCASE LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanRows(rows)
}

func (s *Store) CountPublic(ctx context.Context) (int, error) {
	var n int
	err := s.db.SQL().QueryRowContext(ctx, `
		SELECT COUNT(1) FROM files
		WHERE available = 1 AND (visibility = ? OR visibility = '')`, VisibilityPublic).Scan(&n)
	return n, err
}

func (s *Store) ListPublicPage(ctx context.Context, offset, limit int) ([]Record, error) {
	if limit <= 0 || limit > 80 {
		limit = 20
	}
	if offset < 0 {
		offset = 0
	}
	rows, err := s.db.SQL().QueryContext(ctx, `
		SELECT id, relative_path, filename, mime_type, size, hash_algorithm, hash_digest, modified_at, indexed_at, available, visibility
		FROM files
		WHERE available = 1 AND (visibility = ? OR visibility = '')
		ORDER BY indexed_at DESC, id DESC
		LIMIT ? OFFSET ?`, VisibilityPublic, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanRows(rows)
}

func (s *Store) Summary(ctx context.Context) (Summary, error) {
	var sum Summary
	err := s.db.SQL().QueryRowContext(ctx, `SELECT COUNT(1), COALESCE(SUM(size), 0) FROM files WHERE available = 1`).
		Scan(&sum.Files, &sum.Bytes)
	return sum, err
}

func (s *Store) KnownPaths(ctx context.Context) (map[string]Record, error) {
	rows, err := s.db.SQL().QueryContext(ctx, `
		SELECT id, relative_path, filename, mime_type, size, hash_algorithm, hash_digest, modified_at, indexed_at, available, visibility
		FROM files`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make(map[string]Record)
	for rows.Next() {
		var rec Record
		var modified, indexed string
		var available int
		if err := rows.Scan(&rec.ID, &rec.RelativePath, &rec.Filename, &rec.MIMEType, &rec.Size,
			&rec.Hash.Algorithm, &rec.Hash.Digest, &modified, &indexed, &available, &rec.Visibility); err != nil {
			return nil, err
		}
		rec.Available = available == 1
		rec.ModifiedAt, _ = time.Parse(time.RFC3339, modified)
		rec.IndexedAt, _ = time.Parse(time.RFC3339, indexed)
		out[rec.RelativePath] = rec
	}
	return out, rows.Err()
}

func (s *Store) UpsertDir(ctx context.Context, rec DirRecord) error {
	if rec.ID == "" {
		id, err := newOpaqueID()
		if err != nil {
			return err
		}
		rec.ID = id
	}
	if rec.IndexedAt.IsZero() {
		rec.IndexedAt = time.Now().UTC()
	}
	avail := 0
	if rec.Available {
		avail = 1
	}
	_, err := s.db.SQL().ExecContext(ctx, `
		INSERT INTO directories (id, relative_path, name, indexed_at, available)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(relative_path) DO UPDATE SET
			name = excluded.name,
			indexed_at = excluded.indexed_at,
			available = excluded.available`,
		rec.ID, rec.RelativePath, rec.Name, rec.IndexedAt.UTC().Format(time.RFC3339), avail)
	return err
}

func (s *Store) MarkDirMissing(ctx context.Context, rel string) error {
	_, err := s.db.SQL().ExecContext(ctx, `UPDATE directories SET available = 0, indexed_at = ? WHERE relative_path = ?`,
		time.Now().UTC().Format(time.RFC3339), rel)
	return err
}

func scanRows(rows *sql.Rows) ([]Record, error) {
	var out []Record
	for rows.Next() {
		var rec Record
		var modified, indexed string
		var available int
		if err := rows.Scan(&rec.ID, &rec.RelativePath, &rec.Filename, &rec.MIMEType, &rec.Size,
			&rec.Hash.Algorithm, &rec.Hash.Digest, &modified, &indexed, &available, &rec.Visibility); err != nil {
			return nil, err
		}
		rec.Available = available == 1
		rec.ModifiedAt, _ = time.Parse(time.RFC3339, modified)
		rec.IndexedAt, _ = time.Parse(time.RFC3339, indexed)
		out = append(out, rec)
	}
	return out, rows.Err()
}

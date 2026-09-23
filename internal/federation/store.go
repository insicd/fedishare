package federation

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"

	"github.com/fedishare/fedishare/internal/database"
)

type Store struct {
	db *database.DB
}

func NewStore(db *database.DB) *Store { return &Store{db: db} }

func (s *Store) UpsertFollower(ctx context.Context, f Follower) error {
	now := time.Now().UTC().Format(time.RFC3339)
	if f.CreatedAt == "" {
		f.CreatedAt = now
	}
	accepted := 0
	if f.Accepted {
		accepted = 1
	}
	_, err := s.db.SQL().ExecContext(ctx, `
		INSERT INTO followers (actor_id, inbox_url, shared_inbox_url, username, host, accepted, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(actor_id) DO UPDATE SET
			inbox_url = excluded.inbox_url,
			shared_inbox_url = excluded.shared_inbox_url,
			username = excluded.username,
			host = excluded.host,
			accepted = excluded.accepted,
			updated_at = excluded.updated_at`,
		f.ActorID, f.InboxURL, f.SharedInbox, f.Username, f.Host, accepted, f.CreatedAt, now)
	return err
}

func (s *Store) RemoveFollower(ctx context.Context, actorID string) error {
	_, err := s.db.SQL().ExecContext(ctx, `DELETE FROM followers WHERE actor_id = ?`, actorID)
	return err
}

func (s *Store) GetFollower(ctx context.Context, actorID string) (Follower, bool, error) {
	var f Follower
	var shared sql.NullString
	var accepted int
	err := s.db.SQL().QueryRowContext(ctx, `
		SELECT actor_id, inbox_url, shared_inbox_url, username, host, accepted, created_at
		FROM followers WHERE actor_id = ?`, actorID).
		Scan(&f.ActorID, &f.InboxURL, &shared, &f.Username, &f.Host, &accepted, &f.CreatedAt)
	if err == sql.ErrNoRows {
		return Follower{}, false, nil
	}
	if err != nil {
		return Follower{}, false, err
	}
	f.SharedInbox = shared.String
	f.Accepted = accepted == 1
	return f, true, nil
}

func (s *Store) ListFollowers(ctx context.Context, offset, limit int) ([]Follower, int, error) {
	if limit <= 0 || limit > 80 {
		limit = defaultPage
	}
	if offset < 0 {
		offset = 0
	}
	var total int
	if err := s.db.SQL().QueryRowContext(ctx, `SELECT COUNT(1) FROM followers WHERE accepted = 1`).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := s.db.SQL().QueryContext(ctx, `
		SELECT actor_id, inbox_url, shared_inbox_url, username, host, accepted, created_at
		FROM followers WHERE accepted = 1
		ORDER BY created_at DESC, actor_id
		LIMIT ? OFFSET ?`, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var out []Follower
	for rows.Next() {
		var f Follower
		var shared sql.NullString
		var accepted int
		if err := rows.Scan(&f.ActorID, &f.InboxURL, &shared, &f.Username, &f.Host, &accepted, &f.CreatedAt); err != nil {
			return nil, 0, err
		}
		f.SharedInbox = shared.String
		f.Accepted = accepted == 1
		out = append(out, f)
	}
	return out, total, rows.Err()
}

func (s *Store) AcceptedInboxes(ctx context.Context) ([]Follower, error) {
	list, _, err := s.ListFollowers(ctx, 0, 10000)
	return list, err
}

func (s *Store) CountFollowers(ctx context.Context) (int, error) {
	var n int
	err := s.db.SQL().QueryRowContext(ctx, `SELECT COUNT(1) FROM followers WHERE accepted = 1`).Scan(&n)
	return n, err
}

func (s *Store) ListFollowingIDs(ctx context.Context, offset, limit int) ([]string, int, error) {
	if limit <= 0 {
		limit = defaultPage
	}
	var total int
	if err := s.db.SQL().QueryRowContext(ctx, `SELECT COUNT(1) FROM following WHERE accepted = 1`).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := s.db.SQL().QueryContext(ctx, `
		SELECT actor_id FROM following WHERE accepted = 1
		ORDER BY created_at DESC LIMIT ? OFFSET ?`, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, 0, err
		}
		out = append(out, id)
	}
	return out, total, rows.Err()
}

func (s *Store) UpsertFollowing(ctx context.Context, actorID, inbox string, accepted bool) error {
	now := time.Now().UTC().Format(time.RFC3339)
	acc := 0
	if accepted {
		acc = 1
	}
	_, err := s.db.SQL().ExecContext(ctx, `
		INSERT INTO following (actor_id, inbox_url, accepted, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(actor_id) DO UPDATE SET
			inbox_url = excluded.inbox_url,
			accepted = excluded.accepted,
			updated_at = excluded.updated_at`,
		actorID, inbox, acc, now, now)
	return err
}

func (s *Store) FollowingAccepted(ctx context.Context, actorID string, accepted bool) error {
	acc := 0
	if accepted {
		acc = 1
	}
	_, err := s.db.SQL().ExecContext(ctx, `
		UPDATE following SET accepted = ?, updated_at = ? WHERE actor_id = ?`,
		acc, time.Now().UTC().Format(time.RFC3339), actorID)
	return err
}

func (s *Store) HasOutgoingFollow(ctx context.Context, actorID string) (bool, error) {
	var n int
	err := s.db.SQL().QueryRowContext(ctx, `SELECT COUNT(1) FROM following WHERE actor_id = ?`, actorID).Scan(&n)
	return n > 0, err
}

func (s *Store) Block(ctx context.Context, target, kind string) error {
	_, err := s.db.SQL().ExecContext(ctx, `
		INSERT INTO blocks (target, kind, created_at) VALUES (?, ?, ?)
		ON CONFLICT(target) DO UPDATE SET kind = excluded.kind`,
		target, kind, time.Now().UTC().Format(time.RFC3339))
	return err
}

func (s *Store) Unblock(ctx context.Context, target string) error {
	_, err := s.db.SQL().ExecContext(ctx, `DELETE FROM blocks WHERE target = ?`, target)
	return err
}

func (s *Store) IsBlocked(ctx context.Context, actorID, host string) (bool, error) {
	var n int
	err := s.db.SQL().QueryRowContext(ctx, `
		SELECT COUNT(1) FROM blocks WHERE target = ? OR target = ?`, actorID, host).Scan(&n)
	return n > 0, err
}

func (s *Store) SaveActivity(ctx context.Context, a Activity) error {
	_, err := s.db.SQL().ExecContext(ctx, `
		INSERT INTO activities (id, type, file_id, published_at, payload)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			type = excluded.type,
			file_id = excluded.file_id,
			published_at = excluded.published_at,
			payload = excluded.payload`,
		a.ID, a.Type, a.FileID, a.PublishedAt, a.Payload)
	return err
}

func (s *Store) ListActivities(ctx context.Context, limit int) ([]Activity, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := s.db.SQL().QueryContext(ctx, `
		SELECT id, type, file_id, published_at FROM activities
		ORDER BY published_at DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Activity
	for rows.Next() {
		var a Activity
		var fileID sql.NullString
		if err := rows.Scan(&a.ID, &a.Type, &fileID, &a.PublishedAt); err != nil {
			return nil, err
		}
		a.FileID = fileID.String
		out = append(out, a)
	}
	return out, rows.Err()
}

func (s *Store) Enqueue(ctx context.Context, inboxURL, activityID string, payload []byte) error {
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := s.db.SQL().ExecContext(ctx, `
		INSERT INTO federation_queue (created_at, updated_at, inbox_url, activity_id, payload, attempts, max_attempts, next_attempt_at, status)
		VALUES (?, ?, ?, ?, ?, 0, ?, ?, ?)
		ON CONFLICT(inbox_url, activity_id) DO NOTHING`,
		now, now, inboxURL, activityID, string(payload), maxAttempts, now, StatusPending)
	return err
}

func (s *Store) ClaimDue(ctx context.Context, limit int) ([]Job, error) {
	if limit <= 0 {
		limit = 8
	}
	now := time.Now().UTC().Format(time.RFC3339)
	rows, err := s.db.SQL().QueryContext(ctx, `
		SELECT id, inbox_url, activity_id, payload, attempts, max_attempts, next_attempt_at, COALESCE(last_error, ''), status
		FROM federation_queue
		WHERE status = ? AND next_attempt_at <= ?
		ORDER BY next_attempt_at ASC
		LIMIT ?`, StatusPending, now, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Job
	for rows.Next() {
		var j Job
		var next string
		var payload string
		if err := rows.Scan(&j.ID, &j.InboxURL, &j.ActivityID, &payload, &j.Attempts, &j.MaxAttempts, &next, &j.LastError, &j.Status); err != nil {
			return nil, err
		}
		j.Payload = []byte(payload)
		j.NextAttempt, _ = time.Parse(time.RFC3339, next)
		out = append(out, j)
	}
	return out, rows.Err()
}

func (s *Store) MarkDelivered(ctx context.Context, id int64) error {
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := s.db.SQL().ExecContext(ctx, `
		UPDATE federation_queue SET status = ?, updated_at = ?, last_error = '' WHERE id = ?`,
		StatusDelivered, now, id)
	return err
}

func (s *Store) MarkAttempt(ctx context.Context, id int64, attempts int, next time.Time, lastErr string, status string) error {
	_, err := s.db.SQL().ExecContext(ctx, `
		UPDATE federation_queue SET attempts = ?, next_attempt_at = ?, last_error = ?, status = ?, updated_at = ?
		WHERE id = ?`,
		attempts, next.UTC().Format(time.RFC3339), lastErr, status, time.Now().UTC().Format(time.RFC3339), id)
	return err
}

func (s *Store) PendingCount(ctx context.Context) (int, error) {
	var n int
	err := s.db.SQL().QueryRowContext(ctx, `SELECT COUNT(1) FROM federation_queue WHERE status = ?`, StatusPending).Scan(&n)
	return n, err
}

func (s *Store) SeenSignature(ctx context.Context, hash string) (bool, error) {
	var n int
	err := s.db.SQL().QueryRowContext(ctx, `SELECT COUNT(1) FROM inbox_replay WHERE sig_hash = ?`, hash).Scan(&n)
	return n > 0, err
}

func (s *Store) RememberSignature(ctx context.Context, hash string) error {
	now := time.Now().UTC()
	_, err := s.db.SQL().ExecContext(ctx, `
		INSERT INTO inbox_replay (sig_hash, seen_at) VALUES (?, ?)
		ON CONFLICT(sig_hash) DO NOTHING`, hash, now.Format(time.RFC3339))
	if err != nil {
		return err
	}
	cutoff := now.Add(-replayRetain).Format(time.RFC3339)
	_, _ = s.db.SQL().ExecContext(ctx, `DELETE FROM inbox_replay WHERE seen_at < ?`, cutoff)
	return nil
}

func payloadType(raw []byte) string {
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		return ""
	}
	return typeOf(m["type"])
}

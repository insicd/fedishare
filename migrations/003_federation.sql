-- Followers, outgoing activities, and the durable federation delivery queue.
-- File bytes are never stored here.

CREATE TABLE IF NOT EXISTS followers (
    actor_id TEXT PRIMARY KEY NOT NULL,
    inbox_url TEXT NOT NULL,
    shared_inbox_url TEXT,
    username TEXT,
    host TEXT NOT NULL,
    accepted INTEGER NOT NULL DEFAULT 1,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS followers_host_idx ON followers (host);

CREATE TABLE IF NOT EXISTS following (
    actor_id TEXT PRIMARY KEY NOT NULL,
    inbox_url TEXT NOT NULL,
    accepted INTEGER NOT NULL DEFAULT 0,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS blocks (
    target TEXT PRIMARY KEY NOT NULL,
    kind TEXT NOT NULL,
    created_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS activities (
    id TEXT PRIMARY KEY NOT NULL,
    type TEXT NOT NULL,
    file_id TEXT,
    published_at TEXT NOT NULL,
    payload TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS activities_published_idx ON activities (published_at DESC);

CREATE TABLE IF NOT EXISTS federation_queue (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    inbox_url TEXT NOT NULL,
    activity_id TEXT NOT NULL,
    payload TEXT NOT NULL,
    attempts INTEGER NOT NULL DEFAULT 0,
    max_attempts INTEGER NOT NULL DEFAULT 12,
    next_attempt_at TEXT NOT NULL,
    last_error TEXT,
    status TEXT NOT NULL DEFAULT 'pending',
    UNIQUE (inbox_url, activity_id)
);

CREATE INDEX IF NOT EXISTS federation_queue_due_idx
    ON federation_queue (status, next_attempt_at);

CREATE TABLE IF NOT EXISTS inbox_replay (
    sig_hash TEXT PRIMARY KEY NOT NULL,
    seen_at TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS inbox_replay_seen_idx ON inbox_replay (seen_at);

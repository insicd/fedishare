-- File and directory index. Binary contents stay on disk.
-- Content identity is (hash_algorithm, hash_digest), not the filesystem path.

CREATE TABLE IF NOT EXISTS files (
    id TEXT PRIMARY KEY NOT NULL,
    relative_path TEXT NOT NULL UNIQUE,
    filename TEXT NOT NULL,
    mime_type TEXT NOT NULL,
    size INTEGER NOT NULL,
    hash_algorithm TEXT NOT NULL,
    hash_digest TEXT NOT NULL,
    modified_at TEXT NOT NULL,
    indexed_at TEXT NOT NULL,
    available INTEGER NOT NULL DEFAULT 1,
    visibility TEXT NOT NULL DEFAULT 'public'
);

CREATE INDEX IF NOT EXISTS files_hash_idx ON files (hash_algorithm, hash_digest);
CREATE INDEX IF NOT EXISTS files_available_idx ON files (available);
CREATE INDEX IF NOT EXISTS files_path_idx ON files (relative_path);

CREATE TABLE IF NOT EXISTS directories (
    id TEXT PRIMARY KEY NOT NULL,
    relative_path TEXT NOT NULL UNIQUE,
    name TEXT NOT NULL,
    indexed_at TEXT NOT NULL,
    available INTEGER NOT NULL DEFAULT 1
);

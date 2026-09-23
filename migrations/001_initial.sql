-- Phase 1 foundation. Later phases add files, ActivityPub, and queue tables
-- through additional numbered migrations rather than rewriting this file.

CREATE TABLE IF NOT EXISTS config (
    key TEXT PRIMARY KEY NOT NULL,
    value TEXT NOT NULL,
    updated_at TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS config_updated_at_idx ON config (updated_at);

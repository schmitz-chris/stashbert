-- +goose Up
CREATE TABLE settings (
    key   TEXT PRIMARY KEY,
    value TEXT NOT NULL
) STRICT;

CREATE TABLE members (
    id         TEXT PRIMARY KEY,
    name       TEXT NOT NULL UNIQUE COLLATE NOCASE CHECK (length(name) BETWEEN 1 AND 40),
    created_at TEXT NOT NULL
) STRICT;

CREATE TABLE sessions (
    token_hash   TEXT PRIMARY KEY,
    member_id    TEXT REFERENCES members (id) ON DELETE SET NULL,
    created_at   TEXT NOT NULL,
    last_seen_at TEXT NOT NULL,
    expires_at   TEXT NOT NULL
) STRICT;

-- +goose Down
DROP TABLE sessions;
DROP TABLE members;
DROP TABLE settings;

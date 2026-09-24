-- +goose Up
-- Queue of the MQTT messages (architecture.md, 11.4). seq sets the order of
-- delivery and is the only id that is no UUIDv7. topic is relative to
-- MQTT_TOPIC_PREFIX, payload is JSON.
CREATE TABLE outbox (
    seq        INTEGER PRIMARY KEY AUTOINCREMENT,
    topic      TEXT    NOT NULL,
    payload    TEXT    NOT NULL,
    created_at TEXT    NOT NULL,
    attempts   INTEGER NOT NULL DEFAULT 0 CHECK (attempts >= 0),
    last_error TEXT
) STRICT;

-- +goose Down
DROP TABLE outbox;

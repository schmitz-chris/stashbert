-- name: InsertOutboxEntry :exec
INSERT INTO outbox (topic, payload, created_at)
VALUES (?, ?, ?);

-- name: ListOutboxEntries :many
SELECT * FROM outbox
ORDER BY seq
LIMIT ?;

-- name: DeleteOutboxEntry :exec
DELETE FROM outbox
WHERE seq = ?;

-- name: RecordOutboxFailure :exec
UPDATE outbox
SET attempts = attempts + 1, last_error = ?
WHERE seq = ?;

-- name: DeleteOutboxEntriesBefore :execrows
DELETE FROM outbox
WHERE created_at < ?;

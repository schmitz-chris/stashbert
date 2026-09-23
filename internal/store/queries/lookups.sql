-- name: GetLookup :one
SELECT * FROM lookups
WHERE code = ? AND source = ?;

-- name: UpsertLookup :exec
INSERT INTO lookups (code, source, found, payload, fetched_at)
VALUES (?, ?, ?, ?, ?)
ON CONFLICT (code, source) DO UPDATE SET
    found = excluded.found,
    payload = excluded.payload,
    fetched_at = excluded.fetched_at;

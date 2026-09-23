-- name: ListMembers :many
SELECT id, name, created_at FROM members
ORDER BY name COLLATE NOCASE;

-- name: GetMember :one
SELECT id, name, created_at FROM members
WHERE id = ?;

-- name: CreateMember :exec
INSERT INTO members (id, name, created_at)
VALUES (?, ?, ?);

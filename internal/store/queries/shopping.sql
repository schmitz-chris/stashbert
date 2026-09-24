-- name: SetProductMarked :one
UPDATE products
SET marked = ?, updated_at = ?
WHERE id = ?
RETURNING *;

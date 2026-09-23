-- name: InsertMovement :one
INSERT INTO movements (id, product_id, kind, delta, stock_after, barcode,
    idempotency_key, request_hash, created_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
RETURNING *;

-- name: GetMovementByIdempotencyKey :one
SELECT * FROM movements
WHERE idempotency_key = ?;

-- name: UpdateProductStock :one
UPDATE products
SET stock = ?, updated_at = ?
WHERE id = ?
RETURNING *;

-- name: ListMovements :many
SELECT * FROM movements
ORDER BY id DESC
LIMIT ?;

-- name: ListMovementsBefore :many
SELECT * FROM movements
WHERE id < ?
ORDER BY id DESC
LIMIT ?;

-- name: ListProductMovements :many
SELECT * FROM movements
WHERE product_id = ?
ORDER BY id DESC
LIMIT ?;

-- name: ListProductMovementsBefore :many
SELECT * FROM movements
WHERE product_id = ? AND id < ?
ORDER BY id DESC
LIMIT ?;

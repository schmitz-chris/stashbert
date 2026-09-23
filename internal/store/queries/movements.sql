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

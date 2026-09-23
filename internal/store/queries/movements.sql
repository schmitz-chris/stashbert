-- name: InsertMovement :one
INSERT INTO movements (id, product_id, kind, delta, stock_after, barcode, created_at)
VALUES (?, ?, ?, ?, ?, ?, ?)
RETURNING *;

-- name: UpdateProductStock :one
UPDATE products
SET stock = ?, updated_at = ?
WHERE id = ?
RETURNING *;

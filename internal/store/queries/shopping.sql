-- name: SetProductMarked :one
UPDATE products
SET marked = ?, updated_at = ?
WHERE id = ?
RETURNING *;

-- name: ListShoppingSnapshotProducts :many
SELECT id, name, stock, target, min_stock, marked, crate_size FROM products
WHERE target > 0 OR marked = 1
ORDER BY name COLLATE NOCASE, id;

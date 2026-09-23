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

-- name: ListPendingProducts :many
SELECT id FROM products
WHERE lookup_state = 'pending'
ORDER BY created_at, id
LIMIT ?;

-- name: FirstProductBarcode :one
SELECT code FROM barcodes
WHERE product_id = ?
ORDER BY code
LIMIT 1;

-- name: UpdateProductLookup :exec
UPDATE products
SET name = ?, brand = ?, package_size = ?, origin = ?, image_source_url = ?,
    lookup_state = ?, updated_at = ?
WHERE id = ?;

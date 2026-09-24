-- name: ListProducts :many
SELECT * FROM products
ORDER BY name COLLATE NOCASE, id;

-- name: GetProduct :one
SELECT * FROM products
WHERE id = ?;

-- name: ListBarcodes :many
SELECT * FROM barcodes
ORDER BY code;

-- name: ListProductBarcodes :many
SELECT * FROM barcodes
WHERE product_id = ?
ORDER BY code;

-- name: GetBarcode :one
SELECT * FROM barcodes
WHERE code = ?;

-- name: InsertProduct :one
INSERT INTO products (id, name, brand, package_size, target, min_stock, note,
    origin, lookup_state, needs_review, image_source_url, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
RETURNING *;

-- name: InsertBarcode :one
INSERT INTO barcodes (code, product_id, units, created_at)
VALUES (?, ?, ?, ?)
RETURNING *;

-- name: UpdateProduct :one
UPDATE products
SET name = ?, brand = ?, package_size = ?, target = ?, min_stock = ?, note = ?,
    needs_review = ?, updated_at = ?
WHERE id = ?
RETURNING *;

-- name: DeleteProduct :execrows
DELETE FROM products
WHERE id = ?;

-- name: DeleteProductBarcode :execrows
DELETE FROM barcodes
WHERE code = ? AND product_id = ?;

-- name: MoveProductBarcodes :exec
UPDATE barcodes
SET product_id = sqlc.arg(target_id)
WHERE product_id = sqlc.arg(source_id);

-- name: SetProductImage :one
UPDATE products
SET image_file = ?, image_source_url = NULL, updated_at = ?
WHERE id = ?
RETURNING *;

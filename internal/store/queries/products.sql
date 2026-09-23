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

-- name: InsertProduct :one
INSERT INTO products (id, name, brand, package_size, target, min_stock, note,
    origin, lookup_state, needs_review, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
RETURNING *;

-- name: InsertBarcode :one
INSERT INTO barcodes (code, product_id, units, created_at)
VALUES (?, ?, ?, ?)
RETURNING *;

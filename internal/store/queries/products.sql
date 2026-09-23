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

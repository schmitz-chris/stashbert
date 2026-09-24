-- +goose Up
-- Bottles per crate (ADR-0017), NULL for products that are not bought in
-- crates. Nullable without the keyword NULL, which sqlc cannot parse here.
ALTER TABLE products ADD COLUMN crate_size INTEGER CHECK (crate_size IS NULL OR crate_size BETWEEN 2 AND 100);

-- +goose Down
ALTER TABLE products DROP COLUMN crate_size;

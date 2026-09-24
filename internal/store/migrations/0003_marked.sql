-- +goose Up
-- Marked for shopping (ADR-0015).
ALTER TABLE products ADD COLUMN marked INTEGER NOT NULL DEFAULT 0 CHECK (marked IN (0, 1));

-- +goose Down
ALTER TABLE products DROP COLUMN marked;

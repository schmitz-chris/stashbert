-- +goose Up
CREATE TABLE products (
    id               TEXT    NOT NULL PRIMARY KEY,
    name             TEXT    NOT NULL CHECK (length(name) BETWEEN 1 AND 120),
    brand            TEXT             CHECK (length(brand) <= 120),
    package_size     TEXT             CHECK (length(package_size) <= 40),
    stock            INTEGER NOT NULL DEFAULT 0 CHECK (stock >= 0),
    target           INTEGER NOT NULL DEFAULT 0 CHECK (target >= 0),
    min_stock        INTEGER          CHECK (min_stock >= 0),
    note             TEXT             CHECK (length(note) <= 500),
    origin           TEXT    NOT NULL CHECK (origin IN ('openfoodfacts', 'openbeautyfacts', 'openpetfoodfacts', 'openproductsfacts', 'manual', 'placeholder')),
    lookup_state     TEXT    NOT NULL CHECK (lookup_state IN ('none', 'pending', 'done', 'not_found')),
    needs_review     INTEGER NOT NULL CHECK (needs_review IN (0, 1)),
    image_source_url TEXT,
    image_file       TEXT,
    created_at       TEXT    NOT NULL,
    updated_at       TEXT    NOT NULL,
    CHECK (min_stock <= target)
) STRICT;

CREATE TABLE barcodes (
    code       TEXT    NOT NULL PRIMARY KEY,
    product_id TEXT    NOT NULL REFERENCES products (id) ON DELETE CASCADE,
    units      INTEGER NOT NULL DEFAULT 1 CHECK (units >= 1),
    created_at TEXT    NOT NULL
) STRICT;

CREATE INDEX idx_barcodes_product_id ON barcodes (product_id);

-- Append only. The only exception: merging products moves movements to the target product.
CREATE TABLE movements (
    id              TEXT    NOT NULL PRIMARY KEY,
    product_id      TEXT    NOT NULL REFERENCES products (id) ON DELETE CASCADE,
    kind            TEXT    NOT NULL CHECK (kind IN ('add', 'consume', 'inventory', 'reversal', 'merge')),
    delta           INTEGER NOT NULL,
    stock_after     INTEGER NOT NULL CHECK (stock_after >= 0),
    barcode         TEXT,
    reverses_id     TEXT    UNIQUE REFERENCES movements (id),
    idempotency_key TEXT    UNIQUE,
    request_hash    TEXT,
    created_at      TEXT    NOT NULL
) STRICT;

CREATE INDEX idx_movements_product_id_id ON movements (product_id, id);

-- Cache of external lookups. payload is NULL if the product was not found.
CREATE TABLE lookups (
    code       TEXT    NOT NULL,
    source     TEXT    NOT NULL CHECK (source IN ('off')),
    found      INTEGER NOT NULL CHECK (found IN (0, 1)),
    payload    TEXT,
    fetched_at TEXT    NOT NULL,
    PRIMARY KEY (code, source)
) STRICT;

-- +goose Down
DROP TABLE lookups;
DROP TABLE movements;
DROP TABLE barcodes;
DROP TABLE products;

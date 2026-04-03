-- +goose Up
CREATE TABLE IF NOT EXISTS _nexmap_meta (
    key   TEXT PRIMARY KEY,
    value TEXT NOT NULL
);
INSERT INTO _nexmap_meta (key, value) VALUES ('schema_version', '1');

-- +goose Down
DROP TABLE IF EXISTS _nexmap_meta;

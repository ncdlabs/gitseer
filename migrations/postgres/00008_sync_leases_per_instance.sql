-- +goose Up
CREATE TABLE sync_leases_new (
    id INTEGER PRIMARY KEY,
    holder TEXT NOT NULL,
    expires_at TEXT NOT NULL
);
INSERT INTO sync_leases_new (id, holder, expires_at) SELECT id, holder, expires_at FROM sync_leases;
DROP TABLE sync_leases;
ALTER TABLE sync_leases_new RENAME TO sync_leases;

-- +goose Down
CREATE TABLE sync_leases_old (
    id INTEGER PRIMARY KEY CHECK (id = 1),
    holder TEXT NOT NULL,
    expires_at TEXT NOT NULL
);
INSERT INTO sync_leases_old (id, holder, expires_at)
SELECT id, holder, expires_at FROM sync_leases WHERE id = 1;
DROP TABLE sync_leases;
ALTER TABLE sync_leases_old RENAME TO sync_leases;

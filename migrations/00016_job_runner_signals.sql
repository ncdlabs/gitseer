-- +goose Up
-- Optional forge job labels / freeform message for best-effort runner_unavailable_queued attention.

ALTER TABLE jobs ADD COLUMN labels_json TEXT;
ALTER TABLE jobs ADD COLUMN message TEXT NOT NULL DEFAULT '';

-- +goose Down
-- SQLite cannot DROP COLUMN portably in older versions; leave columns on down.

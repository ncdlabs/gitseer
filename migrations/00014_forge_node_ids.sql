-- +goose Up
ALTER TABLE organizations ADD COLUMN node_id TEXT NOT NULL DEFAULT '';
ALTER TABLE repositories ADD COLUMN node_id TEXT NOT NULL DEFAULT '';
ALTER TABLE pull_requests ADD COLUMN node_id TEXT NOT NULL DEFAULT '';
ALTER TABLE workflow_runs ADD COLUMN node_id TEXT NOT NULL DEFAULT '';
ALTER TABLE jobs ADD COLUMN node_id TEXT NOT NULL DEFAULT '';

-- +goose Down
ALTER TABLE jobs DROP COLUMN node_id;
ALTER TABLE workflow_runs DROP COLUMN node_id;
ALTER TABLE pull_requests DROP COLUMN node_id;
ALTER TABLE repositories DROP COLUMN node_id;
ALTER TABLE organizations DROP COLUMN node_id;

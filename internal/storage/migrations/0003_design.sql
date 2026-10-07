-- M3: AI design requests and the per-project scope that auto mode may use.

-- auto_policy lists what auto mode may do without asking (connections it
-- may use, whether a budget is required). '{}' allows nothing automatic.
ALTER TABLE projects ADD COLUMN auto_policy TEXT NOT NULL DEFAULT '{}';

CREATE TABLE design_requests (
    id            TEXT PRIMARY KEY,
    project_id    TEXT REFERENCES projects (id), -- NULL until a new service is created
    goal          TEXT NOT NULL,
    input         TEXT NOT NULL,                 -- the full request (human tasks, mode, scope)
    mode          TEXT NOT NULL CHECK (mode IN ('review', 'auto')),
    connection_id TEXT REFERENCES provider_connections (id),
    status        TEXT NOT NULL CHECK (status IN ('drafting', 'ready', 'failed', 'applied', 'discarded')),
    proposal      TEXT NOT NULL DEFAULT '{}',
    check_result  TEXT NOT NULL DEFAULT '{}',
    error         TEXT NOT NULL DEFAULT '',
    applied       TEXT NOT NULL DEFAULT '{}',    -- ids created on apply
    created_at    TEXT NOT NULL,
    updated_at    TEXT NOT NULL
);
CREATE INDEX design_requests_created ON design_requests (created_at);

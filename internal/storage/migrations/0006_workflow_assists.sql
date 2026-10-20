-- AI help on one workflow: a proposed edit of its draft (kind "edit",
-- result holds the proposed draft, notes and check issues; applied only
-- when the person accepts it and only onto base_revision), or a pipeline
-- diagram made with the bundled diagram-design skill (kind "diagram",
-- result holds the HTML file's path inside the data folder).
CREATE TABLE workflow_assists (
    id            TEXT PRIMARY KEY,
    project_id    TEXT NOT NULL REFERENCES projects (id),
    workflow_id   TEXT NOT NULL,
    kind          TEXT NOT NULL CHECK (kind IN ('edit', 'diagram')),
    request       TEXT NOT NULL DEFAULT '',
    base_revision INTEGER NOT NULL,
    connection_id TEXT REFERENCES provider_connections (id),
    model         TEXT NOT NULL DEFAULT '',
    status        TEXT NOT NULL CHECK (status IN ('working', 'ready', 'failed', 'applied', 'discarded')),
    result        TEXT NOT NULL DEFAULT '{}',
    error         TEXT NOT NULL DEFAULT '',
    created_at    TEXT NOT NULL,
    updated_at    TEXT NOT NULL,
    FOREIGN KEY (workflow_id, project_id) REFERENCES workflows (id, project_id)
);
CREATE INDEX workflow_assists_workflow ON workflow_assists (workflow_id, created_at);

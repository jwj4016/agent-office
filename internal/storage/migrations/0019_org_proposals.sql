-- Company requests, 4th revision: for a service that already exists, an
-- AI proposes its organization (departments of its assignees, and an
-- intake desk when it has none); the person looks at it and applies it
-- (docs/plan/company-requests/plan.md). The proposal is kept with the
-- call's usage, which counts against the service budget.
--
-- status: working (the AI is writing), ready (waiting for the person),
-- failed, interrupted (the app stopped), applied, discarded.
-- result is the checked plan (domain.OrgPlan); applied keeps what applying
-- created (storage.OrgApplied).
CREATE TABLE org_proposals (
    id             TEXT PRIMARY KEY,
    project_id     TEXT NOT NULL REFERENCES projects (id),
    status         TEXT NOT NULL DEFAULT 'working' CHECK (status IN
                   ('working', 'ready', 'failed', 'interrupted', 'applied', 'discarded')),
    connection_id  TEXT,
    model          TEXT NOT NULL DEFAULT '',
    usage          TEXT NOT NULL DEFAULT '{}',
    usage_reported INTEGER NOT NULL DEFAULT 0 CHECK (usage_reported IN (0, 1)),
    result         TEXT NOT NULL DEFAULT '{}',
    applied        TEXT NOT NULL DEFAULT '{}',
    error          TEXT NOT NULL DEFAULT '',
    started_at     TEXT NOT NULL,
    ended_at       TEXT,
    CHECK (status <> 'failed' OR error <> '')
);
CREATE INDEX org_proposals_project ON org_proposals (project_id, started_at);

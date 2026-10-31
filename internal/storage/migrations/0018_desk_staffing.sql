-- Company requests, 3rd revision: when a request reaches a service (or the
-- company) without an intake desk, an AI looks at the service and writes
-- the new desk's name and instructions; the person can also ask for that,
-- or have the instructions written again (docs/plan/company-requests/
-- plan.md §4.1). Every such call is kept here with the usage it reported:
-- service calls count against the service budget, company calls against
-- the company desk's monthly budget.
--
-- kind: create (a new desk) or rewrite (new instructions for a desk).
-- request_id names the request that made the app staff the desk, if any.
-- result keeps what was written: {"name", "instructions", "assignmentId"}.
CREATE TABLE desk_staffing (
    id              TEXT PRIMARY KEY,
    organization_id TEXT NOT NULL REFERENCES organizations (id),
    project_id      TEXT REFERENCES projects (id),
    request_id      TEXT REFERENCES requests (id),
    kind            TEXT NOT NULL CHECK (kind IN ('create', 'rewrite')),
    connection_id   TEXT,
    model           TEXT NOT NULL DEFAULT '',
    status          TEXT NOT NULL DEFAULT 'working' CHECK (status IN ('working', 'succeeded', 'failed', 'interrupted')),
    usage           TEXT NOT NULL DEFAULT '{}',
    usage_reported  INTEGER NOT NULL DEFAULT 0 CHECK (usage_reported IN (0, 1)),
    result          TEXT NOT NULL DEFAULT '{}',
    error           TEXT NOT NULL DEFAULT '',
    started_at      TEXT NOT NULL,
    ended_at        TEXT,
    CHECK (status <> 'failed' OR error <> '')
);
CREATE INDEX desk_staffing_project ON desk_staffing (project_id, started_at);
CREATE INDEX desk_staffing_request ON desk_staffing (request_id);

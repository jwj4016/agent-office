-- Core entities (spec §5). Child rows that belong to a project reference
-- their parent through (id, project_id) composite keys so the database
-- itself rejects cross-project references (T23).

CREATE TABLE organizations (
    id           TEXT PRIMARY KEY,
    name         TEXT NOT NULL,
    instructions TEXT NOT NULL DEFAULT '',
    policy       TEXT NOT NULL DEFAULT '{}',
    created_at   TEXT NOT NULL
);

CREATE TABLE roles (
    id              TEXT PRIMARY KEY,
    organization_id TEXT NOT NULL REFERENCES organizations (id),
    parent_role_id  TEXT REFERENCES roles (id),
    name            TEXT NOT NULL,
    mission         TEXT NOT NULL DEFAULT '',
    instructions    TEXT NOT NULL DEFAULT '',
    output_defaults TEXT NOT NULL DEFAULT '[]',
    policy          TEXT NOT NULL DEFAULT '{}',
    appearance      TEXT NOT NULL DEFAULT '{}',
    created_at      TEXT NOT NULL,
    updated_at      TEXT NOT NULL,
    CHECK (parent_role_id IS NULL OR parent_role_id <> id)
);
CREATE INDEX roles_org ON roles (organization_id);

CREATE TABLE projects (
    id              TEXT PRIMARY KEY,
    organization_id TEXT NOT NULL REFERENCES organizations (id),
    name            TEXT NOT NULL,
    goal            TEXT NOT NULL DEFAULT '',
    instructions    TEXT NOT NULL DEFAULT '',
    workspace_path  TEXT NOT NULL DEFAULT '',
    budget          TEXT NOT NULL DEFAULT '{}',
    mode            TEXT NOT NULL DEFAULT 'review' CHECK (mode IN ('review', 'auto')),
    status          TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'archived')),
    created_at      TEXT NOT NULL,
    updated_at      TEXT NOT NULL
);

CREATE TABLE provider_connections (
    id                     TEXT PRIMARY KEY,
    name                   TEXT NOT NULL,
    provider               TEXT NOT NULL,
    executable_path        TEXT NOT NULL DEFAULT '',
    secret_ref             TEXT NOT NULL DEFAULT '' CHECK (secret_ref = '' OR secret_ref LIKE 'secret://%'),
    config                 TEXT NOT NULL DEFAULT '{}',
    verified_capabilities  TEXT NOT NULL DEFAULT '{}',
    created_at             TEXT NOT NULL,
    updated_at             TEXT NOT NULL
);

CREATE TABLE assignments (
    id            TEXT PRIMARY KEY,
    project_id    TEXT NOT NULL REFERENCES projects (id),
    role_id       TEXT NOT NULL REFERENCES roles (id),
    actor_kind    TEXT NOT NULL CHECK (actor_kind IN ('ai', 'human')),
    actor_id      TEXT NOT NULL DEFAULT '',
    display_name  TEXT NOT NULL,
    connection_id TEXT REFERENCES provider_connections (id),
    model         TEXT NOT NULL DEFAULT '',
    overrides     TEXT NOT NULL DEFAULT '{}',
    created_at    TEXT NOT NULL,
    updated_at    TEXT NOT NULL,
    UNIQUE (id, project_id),
    CHECK (actor_kind = 'ai' OR (actor_id <> '' AND connection_id IS NULL))
);
CREATE INDEX assignments_project ON assignments (project_id);

CREATE TABLE workflows (
    id         TEXT PRIMARY KEY,
    project_id TEXT NOT NULL REFERENCES projects (id),
    title      TEXT NOT NULL,
    draft_json TEXT NOT NULL,
    revision   INTEGER NOT NULL DEFAULT 0,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    UNIQUE (id, project_id)
);
CREATE INDEX workflows_project ON workflows (project_id);

CREATE TABLE workflow_versions (
    id                   TEXT PRIMARY KEY,
    workflow_id          TEXT NOT NULL,
    project_id           TEXT NOT NULL,
    number               INTEGER NOT NULL,
    spec_json            TEXT NOT NULL,
    assignments_snapshot TEXT NOT NULL,
    policy_snapshot      TEXT NOT NULL,
    created_at           TEXT NOT NULL,
    UNIQUE (workflow_id, number),
    UNIQUE (id, project_id),
    FOREIGN KEY (workflow_id, project_id) REFERENCES workflows (id, project_id)
);

CREATE TABLE runs (
    id                  TEXT PRIMARY KEY,
    project_id          TEXT NOT NULL,
    workflow_version_id TEXT NOT NULL,
    status              TEXT NOT NULL CHECK (status IN
                        ('running', 'waiting', 'paused', 'succeeded', 'failed', 'interrupted', 'cancelled')),
    paused              INTEGER NOT NULL DEFAULT 0 CHECK (paused IN (0, 1)),
    limits              TEXT NOT NULL DEFAULT '{}',
    step_generations    TEXT NOT NULL DEFAULT '{}',
    started_at          TEXT NOT NULL,
    ended_at            TEXT,
    UNIQUE (id, project_id),
    FOREIGN KEY (workflow_version_id, project_id) REFERENCES workflow_versions (id, project_id)
);
CREATE INDEX runs_project ON runs (project_id, status);

CREATE TABLE workspaces (
    id            TEXT PRIMARY KEY,
    project_id    TEXT NOT NULL,
    run_id        TEXT NOT NULL,
    assignment_id TEXT,
    path          TEXT NOT NULL,
    base_commit   TEXT NOT NULL DEFAULT '',
    branch        TEXT NOT NULL DEFAULT '',
    status        TEXT NOT NULL,
    created_at    TEXT NOT NULL,
    UNIQUE (id, project_id),
    FOREIGN KEY (run_id, project_id) REFERENCES runs (id, project_id),
    FOREIGN KEY (assignment_id, project_id) REFERENCES assignments (id, project_id)
);

CREATE TABLE step_attempts (
    id               TEXT PRIMARY KEY,
    project_id       TEXT NOT NULL,
    run_id           TEXT NOT NULL,
    step_id          TEXT NOT NULL,
    revision_round   INTEGER NOT NULL DEFAULT 0,
    attempt          INTEGER NOT NULL DEFAULT 1,
    generation       INTEGER NOT NULL,
    status           TEXT NOT NULL CHECK (status IN
                     ('pending', 'ready', 'running', 'verifying', 'waiting_human', 'waiting_approval',
                      'waiting_input', 'succeeded', 'skipped', 'failed', 'interrupted', 'cancelled')),
    assignment_id    TEXT,
    input_manifest   TEXT NOT NULL DEFAULT '[]',
    provider_session TEXT NOT NULL DEFAULT '',
    workspace_id     TEXT,
    error            TEXT NOT NULL DEFAULT '',
    created_at       TEXT NOT NULL,
    started_at       TEXT,
    ended_at         TEXT,
    UNIQUE (run_id, step_id, generation, attempt),
    UNIQUE (id, project_id),
    FOREIGN KEY (run_id, project_id) REFERENCES runs (id, project_id),
    FOREIGN KEY (assignment_id, project_id) REFERENCES assignments (id, project_id),
    FOREIGN KEY (workspace_id, project_id) REFERENCES workspaces (id, project_id)
);
CREATE INDEX step_attempts_run ON step_attempts (run_id, step_id);

CREATE TABLE artifacts (
    id              TEXT PRIMARY KEY,
    project_id      TEXT NOT NULL,
    run_id          TEXT NOT NULL,
    step_attempt_id TEXT NOT NULL,
    output_key      TEXT NOT NULL,
    version         INTEGER NOT NULL,
    type            TEXT NOT NULL CHECK (type IN ('markdown', 'json', 'file', 'code_change', 'report')),
    path            TEXT NOT NULL,
    hash            TEXT NOT NULL,
    size            INTEGER NOT NULL,
    validity        TEXT NOT NULL DEFAULT 'valid' CHECK (validity IN ('valid', 'stale', 'invalid')),
    source_artifact_id TEXT REFERENCES artifacts (id),
    created_at      TEXT NOT NULL,
    UNIQUE (step_attempt_id, output_key, version),
    UNIQUE (id, project_id),
    FOREIGN KEY (run_id, project_id) REFERENCES runs (id, project_id),
    FOREIGN KEY (step_attempt_id, project_id) REFERENCES step_attempts (id, project_id)
);
CREATE INDEX artifacts_run ON artifacts (run_id, output_key);

CREATE TABLE messages (
    id              TEXT PRIMARY KEY,
    project_id      TEXT NOT NULL,
    run_id          TEXT NOT NULL,
    step_attempt_id TEXT,
    sender          TEXT NOT NULL,
    recipient       TEXT NOT NULL,
    kind            TEXT NOT NULL CHECK (kind IN
                    ('question', 'answer', 'review_request', 'proposal', 'decision', 'handoff', 'escalation')),
    body            TEXT NOT NULL,
    artifact_refs   TEXT NOT NULL DEFAULT '[]',
    reply_to        TEXT,
    created_at      TEXT NOT NULL,
    UNIQUE (id, project_id),
    FOREIGN KEY (run_id, project_id) REFERENCES runs (id, project_id),
    FOREIGN KEY (step_attempt_id, project_id) REFERENCES step_attempts (id, project_id),
    FOREIGN KEY (reply_to, project_id) REFERENCES messages (id, project_id)
);
CREATE INDEX messages_run ON messages (run_id, created_at);

-- One row per approval request. decision is NULL until decided and is
-- written exactly once (UPDATE ... WHERE decision IS NULL). A step attempt
-- has at most one 'step' approval but may raise many tool approvals,
-- told apart by request_key (the provider's request id).
CREATE TABLE approvals (
    id              TEXT PRIMARY KEY,
    project_id      TEXT NOT NULL,
    run_id          TEXT NOT NULL,
    step_attempt_id TEXT NOT NULL,
    generation      INTEGER NOT NULL,
    kind            TEXT NOT NULL CHECK (kind IN ('step', 'tool', 'external')),
    request_key     TEXT NOT NULL DEFAULT '',
    target_manifest TEXT NOT NULL,
    decision        TEXT CHECK (decision IN ('approved', 'rejected', 'held')),
    reason          TEXT NOT NULL DEFAULT '',
    rework_targets  TEXT NOT NULL DEFAULT '[]',
    created_at      TEXT NOT NULL,
    decided_at      TEXT,
    FOREIGN KEY (run_id, project_id) REFERENCES runs (id, project_id),
    FOREIGN KEY (step_attempt_id, project_id) REFERENCES step_attempts (id, project_id),
    UNIQUE (step_attempt_id, kind, request_key),
    CHECK ((decision IS NULL) = (decided_at IS NULL)),
    CHECK (kind <> 'step' OR request_key = '')
);

-- sequence is the global, gap-free-per-database ordering used by the UI.
CREATE TABLE execution_events (
    sequence        INTEGER PRIMARY KEY AUTOINCREMENT,
    id              TEXT NOT NULL UNIQUE,
    project_id      TEXT NOT NULL REFERENCES projects (id),
    run_id          TEXT,
    step_attempt_id TEXT,
    kind            TEXT NOT NULL,
    payload         TEXT NOT NULL DEFAULT '{}',
    created_at      TEXT NOT NULL,
    FOREIGN KEY (run_id, project_id) REFERENCES runs (id, project_id),
    FOREIGN KEY (step_attempt_id, project_id) REFERENCES step_attempts (id, project_id)
);
CREATE INDEX execution_events_run ON execution_events (run_id, sequence);

-- The first release ships with exactly one organization (spec §2.1).
INSERT INTO organizations (id, name, created_at)
VALUES ('org-default', '내 회사', strftime('%Y-%m-%dT%H:%M:%fZ', 'now'));

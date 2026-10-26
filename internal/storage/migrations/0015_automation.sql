-- Service operations stage 7: automatic mode. A work plan may run a
-- confirmed version of an existing workflow instead of a one-step plan,
-- and keeps the start approval it ran under. Work made from events in an
-- auto-mode service gets an auto-start record that the auto starter takes
-- through planning, approval, the run and automatic sharing, or sends to
-- a person for review. A department's lead AI may decide reference
-- collaboration requests; the person confirms or reverts that decision
-- (docs/plan/service-operations/plan.md §4.5, §4.6, §9.1).

-- SQLite cannot add a composite foreign key to an existing table, so the
-- service of workflow_version_id is checked in code when it is written.
-- approved_hash is the work and plan the last start was approved for;
-- any change to them makes the approval void.
ALTER TABLE work_plans ADD COLUMN workflow_version_id TEXT DEFAULT NULL;
ALTER TABLE work_plans ADD COLUMN approved_hash TEXT DEFAULT NULL;
ALTER TABLE work_plans ADD COLUMN approved_by TEXT DEFAULT NULL;
ALTER TABLE work_plans ADD COLUMN approved_at TEXT DEFAULT NULL;

-- 1: a person reviews before every start, even in auto mode.
ALTER TABLE work_items ADD COLUMN review_before_start INTEGER DEFAULT NULL;

-- When the person looked at the lead AI's decision on a request.
ALTER TABLE collab_requests ADD COLUMN reviewed_at TEXT DEFAULT NULL;

-- The history now also keeps a reverted AI decision.
CREATE TABLE collab_request_history_new (
    id               TEXT PRIMARY KEY,
    project_id       TEXT NOT NULL,
    request_id       TEXT NOT NULL,
    action           TEXT NOT NULL CHECK (action IN
        ('requested', 'asked', 'answered', 'accepted', 'declined', 'cancelled', 'required', 'reference', 'replaced', 'reverted')),
    body             TEXT NOT NULL DEFAULT '',
    as_department_id TEXT NOT NULL DEFAULT '',
    author           TEXT NOT NULL,
    created_at       TEXT NOT NULL,
    FOREIGN KEY (request_id, project_id) REFERENCES collab_requests (id, project_id)
);
INSERT INTO collab_request_history_new SELECT id, project_id, request_id, action, body, as_department_id, author, created_at FROM collab_request_history;
DROP TABLE collab_request_history;
ALTER TABLE collab_request_history_new RENAME TO collab_request_history;
CREATE INDEX collab_request_history_request ON collab_request_history (request_id, created_at);

-- status: pending (not looked at), planning (an AI plan is being written),
-- approved (checks passed, start approved), started (run going),
-- finished (run over; note says what was shared), review (a person must
-- look: reason says why), cancelled.
CREATE TABLE auto_starts (
    id             TEXT PRIMARY KEY,
    project_id     TEXT NOT NULL,
    work_item_id   TEXT NOT NULL UNIQUE,
    cause_sequence INTEGER,
    status         TEXT NOT NULL DEFAULT 'pending' CHECK (status IN
                   ('pending', 'planning', 'approved', 'started', 'finished', 'review', 'cancelled')),
    plan_job_id    TEXT,
    run_id         TEXT,
    reason         TEXT NOT NULL DEFAULT '',
    note           TEXT NOT NULL DEFAULT '',
    created_at     TEXT NOT NULL,
    updated_at     TEXT NOT NULL,
    UNIQUE (id, project_id),
    FOREIGN KEY (work_item_id, project_id) REFERENCES work_items (id, project_id),
    FOREIGN KEY (plan_job_id, project_id) REFERENCES work_ai_jobs (id, project_id),
    FOREIGN KEY (run_id, project_id) REFERENCES runs (id, project_id),
    CHECK (status <> 'review' OR reason <> ''),
    CHECK (status NOT IN ('started', 'finished') OR run_id IS NOT NULL)
);
CREATE INDEX auto_starts_open ON auto_starts (status, updated_at);

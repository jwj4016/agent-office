-- Service operations stage 6: collaboration requests. A department asks
-- another for help on one of its work items; the receiving department
-- accepts (its work is created or chosen), asks back, or declines. A
-- required request must be accepted and its work done before the asking
-- work, and the goal above it, can be completed
-- (docs/plan/service-operations/plan.md §4.5, §9.1).

-- follow_up is the receiving department's open question while status is
-- 'question'. decided_by says who accepted or declined: the person acting
-- for the department ('user') or, from stage 7, its lead AI ('ai'), which
-- may only decide reference requests. replaces_id names the required
-- request this one took over from.
CREATE TABLE collab_requests (
    id                  TEXT PRIMARY KEY,
    project_id          TEXT NOT NULL REFERENCES projects (id),
    from_work_item_id   TEXT NOT NULL,
    from_department_id  TEXT NOT NULL,
    to_department_id    TEXT NOT NULL,
    question            TEXT NOT NULL CHECK (question <> ''),
    input_versions      TEXT NOT NULL DEFAULT '[]',
    requirement         TEXT NOT NULL DEFAULT 'reference' CHECK (requirement IN ('reference', 'required')),
    requirement_reason  TEXT NOT NULL DEFAULT '',
    status              TEXT NOT NULL DEFAULT 'requested' CHECK (status IN ('requested', 'question', 'accepted', 'declined', 'cancelled')),
    follow_up           TEXT NOT NULL DEFAULT '',
    decided_by          TEXT NOT NULL DEFAULT '' CHECK (decided_by IN ('', 'user', 'ai')),
    decision_job_id     TEXT,
    decision_reason     TEXT NOT NULL DEFAULT '',
    decided_at          TEXT,
    reverted_at         TEXT,
    revert_reason       TEXT NOT NULL DEFAULT '',
    result_work_item_id TEXT UNIQUE,
    replaces_id         TEXT,
    due_at              TEXT,
    event_sequence      INTEGER,
    created_at          TEXT NOT NULL,
    updated_at          TEXT NOT NULL,
    UNIQUE (id, project_id),
    FOREIGN KEY (from_work_item_id, project_id) REFERENCES work_items (id, project_id),
    FOREIGN KEY (from_department_id, project_id) REFERENCES departments (id, project_id),
    FOREIGN KEY (to_department_id, project_id) REFERENCES departments (id, project_id),
    FOREIGN KEY (result_work_item_id, project_id) REFERENCES work_items (id, project_id),
    FOREIGN KEY (decision_job_id, project_id) REFERENCES work_ai_jobs (id, project_id),
    FOREIGN KEY (replaces_id, project_id) REFERENCES collab_requests (id, project_id),
    CHECK (from_department_id <> to_department_id),
    CHECK (decided_by <> 'ai' OR requirement = 'reference'),
    CHECK (status <> 'accepted' OR result_work_item_id IS NOT NULL),
    CHECK ((status = 'question') = (follow_up <> '')),
    CHECK ((status IN ('requested', 'question')) = (decided_at IS NULL))
);
CREATE INDEX collab_requests_from ON collab_requests (from_work_item_id);
CREATE INDEX collab_requests_open ON collab_requests (project_id, status);

-- Everything that happened to a request, oldest first. as_department_id
-- is the department the person (or AI) acted for; '' for the person in
-- charge of the asking work.
CREATE TABLE collab_request_history (
    id               TEXT PRIMARY KEY,
    project_id       TEXT NOT NULL,
    request_id       TEXT NOT NULL,
    action           TEXT NOT NULL CHECK (action IN
        ('requested', 'asked', 'answered', 'accepted', 'declined', 'cancelled', 'required', 'reference', 'replaced')),
    body             TEXT NOT NULL DEFAULT '',
    as_department_id TEXT NOT NULL DEFAULT '',
    author           TEXT NOT NULL,
    created_at       TEXT NOT NULL,
    FOREIGN KEY (request_id, project_id) REFERENCES collab_requests (id, project_id)
);
CREATE INDEX collab_request_history_request ON collab_request_history (request_id, created_at);

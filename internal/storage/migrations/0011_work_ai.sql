-- Service operations stage 3-2: AI work done outside workflow runs
-- (work plans now; department judgments and collaboration decisions in
-- later stages). A job is one logical request; an attempt is one call to
-- a provider. Every attempt keeps the usage its provider reported so the
-- service budget counts failed, interrupted and retried calls too
-- (docs/plan/service-operations/plan.md §4.3, §4.7, §9.1).

CREATE TABLE work_ai_jobs (
    id             TEXT PRIMARY KEY,
    project_id     TEXT NOT NULL REFERENCES projects (id),
    kind           TEXT NOT NULL CHECK (kind IN ('plan', 'judgment', 'collab_decision')),
    lane           TEXT NOT NULL CHECK (lane IN ('interactive', 'background')),
    request_key    TEXT NOT NULL,
    work_item_id   TEXT,
    delivery_id    TEXT,
    round          INTEGER,
    assignment_id  TEXT,
    connection_id  TEXT REFERENCES provider_connections (id),
    model          TEXT NOT NULL DEFAULT '',
    status         TEXT NOT NULL DEFAULT 'pending' CHECK (status IN
                   ('pending', 'working', 'waiting_answer', 'ready', 'held', 'failed')),
    input_snapshot TEXT NOT NULL DEFAULT '{}',
    input_hash     TEXT NOT NULL DEFAULT '',
    result         TEXT NOT NULL DEFAULT '{}',
    error          TEXT NOT NULL DEFAULT '',
    hold_reason    TEXT NOT NULL DEFAULT '',
    -- First call plus two automatic retries; a person's retry adds one.
    max_attempts   INTEGER NOT NULL DEFAULT 3 CHECK (max_attempts >= 1),
    applied_at     TEXT,
    created_at     TEXT NOT NULL,
    updated_at     TEXT NOT NULL,
    UNIQUE (project_id, request_key),
    UNIQUE (delivery_id, round),
    UNIQUE (id, project_id),
    FOREIGN KEY (work_item_id, project_id) REFERENCES work_items (id, project_id),
    FOREIGN KEY (delivery_id, project_id) REFERENCES event_deliveries (id, project_id),
    FOREIGN KEY (assignment_id, project_id) REFERENCES assignments (id, project_id),
    CHECK (kind <> 'plan' OR (work_item_id IS NOT NULL AND delivery_id IS NULL AND round IS NULL)),
    CHECK (kind = 'plan' OR (delivery_id IS NOT NULL AND round IN (1, 2))),
    CHECK (status <> 'failed' OR error <> ''),
    CHECK (status <> 'held' OR hold_reason <> '')
);
CREATE INDEX work_ai_jobs_queue ON work_ai_jobs (lane, status, created_at);
CREATE INDEX work_ai_jobs_work_item ON work_ai_jobs (work_item_id, created_at);

-- usage is the latest running total the provider reported in this
-- attempt; usage_reported stays 0 when it reported nothing, which the
-- budget counts as unknown cost rather than free.
CREATE TABLE work_ai_attempts (
    id             TEXT PRIMARY KEY,
    project_id     TEXT NOT NULL,
    job_id         TEXT NOT NULL,
    number         INTEGER NOT NULL CHECK (number >= 1),
    connection_id  TEXT,
    model          TEXT NOT NULL DEFAULT '',
    status         TEXT NOT NULL DEFAULT 'working' CHECK (status IN ('working', 'succeeded', 'failed', 'interrupted')),
    usage          TEXT NOT NULL DEFAULT '{}',
    usage_reported INTEGER NOT NULL DEFAULT 0 CHECK (usage_reported IN (0, 1)),
    error          TEXT NOT NULL DEFAULT '',
    started_at     TEXT NOT NULL,
    ended_at       TEXT,
    UNIQUE (job_id, number),
    FOREIGN KEY (job_id, project_id) REFERENCES work_ai_jobs (id, project_id)
);
CREATE INDEX work_ai_attempts_project ON work_ai_attempts (project_id);

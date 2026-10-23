-- Service operations stage 2: what each department wants to hear about,
-- and one durable delivery per (event, department, purpose) so a
-- department's inbox survives restarts and is never filled twice
-- (docs/plan/service-operations/plan.md §4.2, §4.3, §4.7, §9.1).

-- event_kinds lists the work/document event kinds the department
-- subscribes to; categories narrows them ('[]' means every category).
-- guidance is the point of view the department judges from.
CREATE TABLE subscriptions (
    id            TEXT PRIMARY KEY,
    project_id    TEXT NOT NULL,
    department_id TEXT NOT NULL,
    event_kinds   TEXT NOT NULL DEFAULT '[]',
    categories    TEXT NOT NULL DEFAULT '[]',
    guidance      TEXT NOT NULL DEFAULT '',
    enabled       INTEGER NOT NULL DEFAULT 1 CHECK (enabled IN (0, 1)),
    created_at    TEXT NOT NULL,
    updated_at    TEXT NOT NULL,
    UNIQUE (id, project_id),
    FOREIGN KEY (department_id, project_id) REFERENCES departments (id, project_id)
);
CREATE INDEX subscriptions_department ON subscriptions (department_id);

-- A delivery is created in the transaction that stores its event.
-- 'held' waits for a person (e.g. the per-event department limit);
-- 'processing' rows left by a crash go back to 'pending' on start.
CREATE TABLE event_deliveries (
    id              TEXT PRIMARY KEY,
    project_id      TEXT NOT NULL,
    event_sequence  INTEGER NOT NULL REFERENCES execution_events (sequence),
    department_id   TEXT NOT NULL,
    subscription_id TEXT,
    purpose         TEXT NOT NULL CHECK (purpose IN ('judge', 'notify', 'collab_request')),
    status          TEXT NOT NULL DEFAULT 'pending' CHECK (status IN
                    ('pending', 'processing', 'waiting_answer', 'done', 'skipped', 'held', 'failed')),
    note            TEXT NOT NULL DEFAULT '',
    error           TEXT NOT NULL DEFAULT '',
    hold_reason     TEXT NOT NULL DEFAULT '',
    created_at      TEXT NOT NULL,
    updated_at      TEXT NOT NULL,
    UNIQUE (event_sequence, department_id, purpose),
    UNIQUE (id, project_id),
    FOREIGN KEY (department_id, project_id) REFERENCES departments (id, project_id),
    FOREIGN KEY (subscription_id, project_id) REFERENCES subscriptions (id, project_id),
    CHECK (status <> 'failed' OR error <> ''),
    CHECK (status <> 'held' OR hold_reason <> '')
);
CREATE INDEX event_deliveries_status ON event_deliveries (status, event_sequence);
CREATE INDEX event_deliveries_department ON event_deliveries (department_id, event_sequence);

-- Work events are looked up by kind within a service.
CREATE INDEX execution_events_project_kind ON execution_events (project_id, kind);

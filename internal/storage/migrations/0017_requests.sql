-- Company requests: a person asks the company or one of its services in
-- words, and an intake desk (an AI) decides what to do with it: answer,
-- ask back, pass it to services, propose work, or propose an action the
-- app carries out once the person approves (docs/plan/company-requests/
-- plan.md §4, §9.1).
--
-- Requests keep their own AI attempts instead of using work_ai_jobs, whose
-- kind and delivery checks would need that table, referenced by several
-- others, to be rebuilt. A request is its own job: it is claimed from
-- 'received' to 'triaging', and each provider call is a request_attempts
-- row that keeps the usage reported (service requests count against the
-- service budget; company requests against the desk's monthly budget).

-- One desk per company (project_id NULL) and one per service. A service
-- desk is one of the service's AI assignments; a company desk has no
-- assignment, so it names its connection and model itself.
CREATE TABLE intake_desks (
    id              TEXT PRIMARY KEY,
    organization_id TEXT NOT NULL REFERENCES organizations (id),
    project_id      TEXT REFERENCES projects (id),
    name            TEXT NOT NULL DEFAULT '',
    assignment_id   TEXT,
    connection_id   TEXT REFERENCES provider_connections (id),
    model           TEXT NOT NULL DEFAULT '',
    instructions    TEXT NOT NULL DEFAULT '',
    budget          TEXT NOT NULL DEFAULT '{}',
    enabled         INTEGER NOT NULL DEFAULT 1 CHECK (enabled IN (0, 1)),
    created_at      TEXT NOT NULL,
    updated_at      TEXT NOT NULL,
    FOREIGN KEY (assignment_id, project_id) REFERENCES assignments (id, project_id),
    CHECK (project_id IS NULL OR assignment_id IS NOT NULL),
    CHECK (project_id IS NOT NULL OR assignment_id IS NULL)
);
CREATE UNIQUE INDEX intake_desks_company ON intake_desks (organization_id) WHERE project_id IS NULL;
CREATE UNIQUE INDEX intake_desks_service ON intake_desks (project_id) WHERE project_id IS NOT NULL;

-- status: received (waiting for its desk), waiting_desk (the service has
-- no usable desk), held (budget or archived service), triaging (an AI
-- call is going), question (the desk asked back), manual (asked back
-- twice: a person handles it), proposed (items wait for the person),
-- routed (passed to services; done when they are), in_progress (actions
-- approved and running), done, declined, cancelled, failed.
-- round is 1, or 2 after the person answered a question.
CREATE TABLE requests (
    id               TEXT PRIMARY KEY,
    organization_id  TEXT NOT NULL REFERENCES organizations (id),
    project_id       TEXT REFERENCES projects (id),
    parent_id        TEXT REFERENCES requests (id),
    parent_item_id   TEXT UNIQUE,
    previous_id      TEXT REFERENCES requests (id),
    body             TEXT NOT NULL CHECK (body <> ''),
    status           TEXT NOT NULL DEFAULT 'received' CHECK (status IN
                     ('received', 'waiting_desk', 'held', 'triaging', 'question', 'manual', 'proposed', 'routed',
                      'in_progress', 'done', 'declined', 'cancelled', 'failed')),
    outcome          TEXT NOT NULL DEFAULT '' CHECK (outcome IN ('', 'answer', 'route', 'work', 'action', 'question', 'decline')),
    round            INTEGER NOT NULL DEFAULT 1 CHECK (round IN (1, 2)),
    answer           TEXT NOT NULL DEFAULT '',
    question         TEXT NOT NULL DEFAULT '',
    reply            TEXT NOT NULL DEFAULT '',
    reason           TEXT NOT NULL DEFAULT '',
    error            TEXT NOT NULL DEFAULT '',
    hold_reason      TEXT NOT NULL DEFAULT '',
    -- First call plus two automatic retries per round; a person's retry
    -- adds one.
    max_attempts     INTEGER NOT NULL DEFAULT 3 CHECK (max_attempts >= 1),
    created_at       TEXT NOT NULL,
    updated_at       TEXT NOT NULL,
    closed_at        TEXT,
    CHECK (parent_id IS NULL OR project_id IS NOT NULL),
    CHECK (status <> 'declined' OR reason <> ''),
    CHECK (status <> 'failed' OR error <> ''),
    CHECK (status NOT IN ('waiting_desk', 'held') OR hold_reason <> ''),
    CHECK ((status IN ('done', 'declined', 'cancelled', 'failed')) = (closed_at IS NOT NULL))
);
CREATE INDEX requests_queue ON requests (status, created_at);
CREATE INDEX requests_project ON requests (project_id, created_at);
CREATE INDEX requests_parent ON requests (parent_id);

-- One provider call for a request. usage is the latest running total
-- reported in the attempt; usage_reported stays 0 when nothing was, which
-- counts as unknown cost rather than free.
CREATE TABLE request_attempts (
    id             TEXT PRIMARY KEY,
    request_id     TEXT NOT NULL REFERENCES requests (id),
    project_id     TEXT REFERENCES projects (id),
    organization_id TEXT NOT NULL REFERENCES organizations (id),
    round          INTEGER NOT NULL CHECK (round IN (1, 2)),
    number         INTEGER NOT NULL CHECK (number >= 1),
    connection_id  TEXT,
    model          TEXT NOT NULL DEFAULT '',
    status         TEXT NOT NULL DEFAULT 'working' CHECK (status IN ('working', 'succeeded', 'failed', 'interrupted')),
    usage          TEXT NOT NULL DEFAULT '{}',
    usage_reported INTEGER NOT NULL DEFAULT 0 CHECK (usage_reported IN (0, 1)),
    result         TEXT NOT NULL DEFAULT '',
    error          TEXT NOT NULL DEFAULT '',
    started_at     TEXT NOT NULL,
    ended_at       TEXT,
    UNIQUE (request_id, round, number)
);
CREATE INDEX request_attempts_project ON request_attempts (project_id);
CREATE INDEX request_attempts_company ON request_attempts (organization_id, started_at) WHERE project_id IS NULL;

-- What a desk's decision asks for: a route to a service (company desk),
-- work to register, or an action for the app to carry out (service desk).
-- status: proposed (the person decides), accepted (work registered),
-- rejected, running (an approved action is being carried out), done,
-- failed (an action failed; it can run again), check (the app stopped
-- while the action ran: the person checks what happened).
CREATE TABLE request_items (
    id               TEXT PRIMARY KEY,
    request_id       TEXT NOT NULL REFERENCES requests (id),
    project_id       TEXT REFERENCES projects (id),
    round            INTEGER NOT NULL CHECK (round IN (1, 2)),
    position         INTEGER NOT NULL CHECK (position BETWEEN 1 AND 5),
    kind             TEXT NOT NULL CHECK (kind IN ('route', 'work', 'action')),
    payload          TEXT NOT NULL DEFAULT '{}',
    status           TEXT NOT NULL DEFAULT 'proposed' CHECK (status IN
                     ('proposed', 'accepted', 'rejected', 'running', 'done', 'failed', 'check')),
    work_item_id     TEXT UNIQUE,
    child_request_id TEXT UNIQUE REFERENCES requests (id),
    result           TEXT NOT NULL DEFAULT '{}',
    error            TEXT NOT NULL DEFAULT '',
    decided_by       TEXT NOT NULL DEFAULT '' CHECK (decided_by IN ('', 'user', 'auto')),
    decision_note    TEXT NOT NULL DEFAULT '',
    decided_at       TEXT,
    created_at       TEXT NOT NULL,
    updated_at       TEXT NOT NULL,
    UNIQUE (request_id, round, position),
    FOREIGN KEY (work_item_id, project_id) REFERENCES work_items (id, project_id),
    CHECK (kind <> 'work' OR (status = 'accepted') = (work_item_id IS NOT NULL)),
    CHECK (kind <> 'route' OR child_request_id IS NOT NULL),
    CHECK (status <> 'failed' OR error <> '')
);
CREATE INDEX request_items_open ON request_items (status, kind);

-- The request work was registered from (checked in code: SQLite cannot add
-- a composite foreign key to an existing table).
ALTER TABLE work_items ADD COLUMN origin_request_id TEXT DEFAULT NULL;

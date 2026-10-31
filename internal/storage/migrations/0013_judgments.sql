-- Service operations stage 5: a department's judgment of a delivery and
-- the work it proposes. A judgment is stored with its AI job's result in
-- one transaction; round 2 exists only after a person answered the
-- question of round 1 (docs/plan/service-operations/plan.md §4.3, §9.1).

-- question_status: '' without a question; 'open' until answered;
-- 'answered' once round 2 is queued; 'manual' when round 2 asked again
-- and a person decides; 'closed' when the person closed it.
CREATE TABLE judgments (
    id              TEXT PRIMARY KEY,
    project_id      TEXT NOT NULL,
    delivery_id     TEXT NOT NULL,
    round           INTEGER NOT NULL CHECK (round IN (1, 2)),
    ai_job_id       TEXT NOT NULL UNIQUE,
    department_id   TEXT NOT NULL,
    outcome         TEXT NOT NULL CHECK (outcome IN ('none', 'propose', 'question')),
    reason          TEXT NOT NULL DEFAULT '',
    evidence        TEXT NOT NULL DEFAULT '[]',
    question        TEXT NOT NULL DEFAULT '',
    answer          TEXT NOT NULL DEFAULT '',
    question_status TEXT NOT NULL DEFAULT '' CHECK (question_status IN ('', 'open', 'answered', 'manual', 'closed')),
    created_at      TEXT NOT NULL,
    updated_at      TEXT NOT NULL,
    UNIQUE (delivery_id, round),
    UNIQUE (id, project_id),
    FOREIGN KEY (delivery_id, project_id) REFERENCES event_deliveries (id, project_id),
    FOREIGN KEY (ai_job_id, project_id) REFERENCES work_ai_jobs (id, project_id),
    FOREIGN KEY (department_id, project_id) REFERENCES departments (id, project_id),
    CHECK ((outcome = 'question') = (question <> '')),
    CHECK ((outcome = 'question') = (question_status <> ''))
);
CREATE INDEX judgments_questions ON judgments (project_id, question_status);

-- A work item a judgment proposes. Accepting registers the work once;
-- duplicate_of names an open work item or proposal of the same
-- department with the same normalized title (a hint, never a block).
CREATE TABLE work_proposals (
    id            TEXT PRIMARY KEY,
    project_id    TEXT NOT NULL,
    judgment_id   TEXT NOT NULL,
    position      INTEGER NOT NULL CHECK (position BETWEEN 1 AND 5),
    department_id TEXT NOT NULL,
    title         TEXT NOT NULL CHECK (title <> ''),
    description   TEXT NOT NULL DEFAULT '',
    done_criteria TEXT NOT NULL DEFAULT '',
    categories    TEXT NOT NULL DEFAULT '[]',
    priority      TEXT NOT NULL DEFAULT 'normal' CHECK (priority IN ('urgent', 'high', 'normal', 'low')),
    status        TEXT NOT NULL DEFAULT 'proposed' CHECK (status IN ('proposed', 'accepted', 'rejected')),
    work_item_id  TEXT UNIQUE,
    duplicate_of  TEXT NOT NULL DEFAULT '',
    decision_note TEXT NOT NULL DEFAULT '',
    decided_at    TEXT,
    created_at    TEXT NOT NULL,
    UNIQUE (judgment_id, position),
    UNIQUE (id, project_id),
    FOREIGN KEY (judgment_id, project_id) REFERENCES judgments (id, project_id),
    FOREIGN KEY (department_id, project_id) REFERENCES departments (id, project_id),
    FOREIGN KEY (work_item_id, project_id) REFERENCES work_items (id, project_id),
    CHECK ((status = 'accepted') = (work_item_id IS NOT NULL)),
    CHECK ((status = 'proposed') = (decided_at IS NULL))
);
CREATE INDEX work_proposals_open ON work_proposals (project_id, status);

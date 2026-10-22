-- Service operations stage 1: departments inside each service, company
-- department templates they can be copied from, per-service categories,
-- and work items with shared goals
-- (docs/plan/service-operations/plan.md §4.1, §9.1).

-- A template is a company-wide starting point for a department: its name,
-- responsibility and the roles that usually work in it. Copying it into a
-- service makes an independent department; later template edits do not
-- change departments made from it.
CREATE TABLE department_templates (
    id              TEXT PRIMARY KEY,
    organization_id TEXT NOT NULL REFERENCES organizations (id),
    name            TEXT NOT NULL CHECK (name <> ''),
    responsibility  TEXT NOT NULL DEFAULT '',
    created_at      TEXT NOT NULL,
    updated_at      TEXT NOT NULL,
    UNIQUE (organization_id, name)
);

CREATE TABLE department_template_roles (
    template_id TEXT NOT NULL REFERENCES department_templates (id) ON DELETE CASCADE,
    role_id     TEXT NOT NULL REFERENCES roles (id) ON DELETE CASCADE,
    PRIMARY KEY (template_id, role_id)
);

-- A department belongs to one service (a service is one company). The
-- lead is one of its members; the composite keys let the database reject
-- another service's assignment.
CREATE TABLE departments (
    id                 TEXT PRIMARY KEY,
    project_id         TEXT NOT NULL REFERENCES projects (id),
    name               TEXT NOT NULL CHECK (name <> ''),
    responsibility     TEXT NOT NULL DEFAULT '',
    lead_assignment_id TEXT,
    template_id        TEXT REFERENCES department_templates (id) ON DELETE SET NULL,
    automation         TEXT NOT NULL DEFAULT '{}',
    limits             TEXT NOT NULL DEFAULT '{}',
    status             TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'inactive')),
    created_at         TEXT NOT NULL,
    updated_at         TEXT NOT NULL,
    UNIQUE (project_id, name),
    UNIQUE (id, project_id),
    FOREIGN KEY (lead_assignment_id, project_id) REFERENCES assignments (id, project_id)
);

-- An assignment belongs to at most one department of its service.
CREATE TABLE department_members (
    assignment_id TEXT PRIMARY KEY,
    project_id    TEXT NOT NULL,
    department_id TEXT NOT NULL,
    created_at    TEXT NOT NULL,
    FOREIGN KEY (assignment_id, project_id) REFERENCES assignments (id, project_id),
    FOREIGN KEY (department_id, project_id) REFERENCES departments (id, project_id)
);
CREATE INDEX department_members_department ON department_members (department_id);

CREATE TABLE project_categories (
    project_id TEXT NOT NULL REFERENCES projects (id),
    name       TEXT NOT NULL,
    position   INTEGER NOT NULL,
    PRIMARY KEY (project_id, name)
);

-- Existing services start with the default list.
WITH defaults (name, position) AS (
    VALUES ('기획', 1), ('디자인', 2), ('개발', 3), ('비용', 4), ('법무', 5), ('운영', 6), ('기타', 7)
)
INSERT INTO project_categories (project_id, name, position)
SELECT p.id, d.name, d.position FROM projects p CROSS JOIN defaults d;

-- Work stays until it is resolved. A goal (kind 'goal') is the parent of
-- other work and always has a responsible department; goals are not
-- nested. Closed items keep closed_at until they are reopened.
CREATE TABLE work_items (
    id                     TEXT PRIMARY KEY,
    project_id             TEXT NOT NULL REFERENCES projects (id),
    kind                   TEXT NOT NULL CHECK (kind IN ('goal', 'work')),
    parent_id              TEXT,
    department_id          TEXT,
    assignee_assignment_id TEXT,
    title                  TEXT NOT NULL CHECK (title <> ''),
    description            TEXT NOT NULL DEFAULT '',
    done_criteria          TEXT NOT NULL DEFAULT '',
    priority               TEXT NOT NULL DEFAULT 'normal' CHECK (priority IN ('urgent', 'high', 'normal', 'low')),
    status                 TEXT NOT NULL DEFAULT 'received' CHECK (status IN
                           ('received', 'ready', 'in_progress', 'waiting', 'blocked', 'done', 'cancelled')),
    status_reason          TEXT NOT NULL DEFAULT '',
    categories             TEXT NOT NULL DEFAULT '[]',
    due_at                 TEXT,
    origin_event_sequence  INTEGER,
    origin_proposal_id     TEXT,
    created_at             TEXT NOT NULL,
    updated_at             TEXT NOT NULL,
    closed_at              TEXT,
    UNIQUE (id, project_id),
    FOREIGN KEY (parent_id, project_id) REFERENCES work_items (id, project_id),
    FOREIGN KEY (department_id, project_id) REFERENCES departments (id, project_id),
    FOREIGN KEY (assignee_assignment_id, project_id) REFERENCES assignments (id, project_id),
    CHECK (status NOT IN ('waiting', 'blocked') OR status_reason <> ''),
    CHECK ((status IN ('done', 'cancelled')) = (closed_at IS NOT NULL)),
    CHECK (kind <> 'goal' OR (parent_id IS NULL AND department_id IS NOT NULL)),
    CHECK (parent_id IS NULL OR parent_id <> id)
);
CREATE INDEX work_items_project ON work_items (project_id, status);
CREATE INDEX work_items_parent ON work_items (parent_id);
CREATE INDEX work_items_department ON work_items (department_id);

-- A work item's history: people's notes and every status change.
CREATE TABLE work_item_notes (
    id           TEXT PRIMARY KEY,
    project_id   TEXT NOT NULL,
    work_item_id TEXT NOT NULL,
    kind         TEXT NOT NULL CHECK (kind IN ('note', 'status')),
    body         TEXT NOT NULL DEFAULT '',
    from_status  TEXT NOT NULL DEFAULT '',
    to_status    TEXT NOT NULL DEFAULT '',
    author       TEXT NOT NULL,
    created_at   TEXT NOT NULL,
    FOREIGN KEY (work_item_id, project_id) REFERENCES work_items (id, project_id),
    CHECK (kind <> 'note' OR body <> ''),
    CHECK (kind <> 'status' OR to_status <> '')
);
CREATE INDEX work_item_notes_item ON work_item_notes (work_item_id, created_at);

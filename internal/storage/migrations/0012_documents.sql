-- Service operations stage 4: shared documents. A document groups the
-- versions of one piece of material (plan 1, plan 2, ...); each version
-- is a read-only copy in the data folder whose content never changes.
-- Confirming a version changes only its review stage. Work items link
-- versions as references, and a run pins the versions it was given
-- (docs/plan/service-operations/plan.md §4.4, §9.1).

-- scope 'departments' limits who may see, link and be told about the
-- document to scope_departments (department ids of the same service).
CREATE TABLE shared_documents (
    id                  TEXT PRIMARY KEY,
    project_id          TEXT NOT NULL REFERENCES projects (id),
    title               TEXT NOT NULL CHECK (title <> ''),
    categories          TEXT NOT NULL DEFAULT '[]',
    owner_department_id TEXT,
    work_item_id        TEXT,
    scope               TEXT NOT NULL DEFAULT 'service' CHECK (scope IN ('service', 'departments')),
    scope_departments   TEXT NOT NULL DEFAULT '[]',
    status              TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'withdrawn')),
    created_at          TEXT NOT NULL,
    updated_at          TEXT NOT NULL,
    UNIQUE (id, project_id),
    FOREIGN KEY (owner_department_id, project_id) REFERENCES departments (id, project_id),
    FOREIGN KEY (work_item_id, project_id) REFERENCES work_items (id, project_id)
);
CREATE INDEX shared_documents_project ON shared_documents (project_id, updated_at);

-- source_kind says where the copy came from: a run's artifact
-- (source_artifact_id) or a file in the service's work folder
-- (source_path, relative to that folder). path is the copy, relative to
-- the data folder.
CREATE TABLE shared_document_versions (
    id                 TEXT PRIMARY KEY,
    document_id        TEXT NOT NULL,
    project_id         TEXT NOT NULL,
    number             INTEGER NOT NULL CHECK (number >= 1),
    stage              TEXT NOT NULL DEFAULT 'draft' CHECK (stage IN ('draft', 'final')),
    confirmed_by       TEXT NOT NULL DEFAULT '',
    confirmed_at       TEXT,
    confirmation_note  TEXT NOT NULL DEFAULT '',
    source_kind        TEXT NOT NULL CHECK (source_kind IN ('artifact', 'workspace')),
    source_artifact_id TEXT,
    source_path        TEXT NOT NULL DEFAULT '',
    file_name          TEXT NOT NULL,
    path               TEXT NOT NULL,
    hash               TEXT NOT NULL,
    size               INTEGER NOT NULL,
    withdrawn_at       TEXT,
    event_sequence     INTEGER,
    created_at         TEXT NOT NULL,
    UNIQUE (document_id, number),
    UNIQUE (id, project_id),
    FOREIGN KEY (document_id, project_id) REFERENCES shared_documents (id, project_id),
    FOREIGN KEY (source_artifact_id, project_id) REFERENCES artifacts (id, project_id),
    CHECK ((stage = 'final') = (confirmed_at IS NOT NULL)),
    CHECK (source_kind <> 'artifact' OR source_artifact_id IS NOT NULL),
    CHECK (source_kind <> 'workspace' OR source_path <> '')
);

-- A work item's reference to a version. Unlinking keeps the row.
CREATE TABLE document_links (
    id           TEXT PRIMARY KEY,
    project_id   TEXT NOT NULL,
    work_item_id TEXT NOT NULL,
    version_id   TEXT NOT NULL,
    linked_at    TEXT NOT NULL,
    unlinked_at  TEXT,
    UNIQUE (work_item_id, version_id),
    FOREIGN KEY (work_item_id, project_id) REFERENCES work_items (id, project_id),
    FOREIGN KEY (version_id, project_id) REFERENCES shared_document_versions (id, project_id)
);

-- The versions a run was given, fixed when it started.
CREATE TABLE document_pins (
    id           TEXT PRIMARY KEY,
    project_id   TEXT NOT NULL,
    work_item_id TEXT NOT NULL,
    version_id   TEXT NOT NULL,
    run_id       TEXT NOT NULL,
    pinned_at    TEXT NOT NULL,
    UNIQUE (run_id, version_id),
    FOREIGN KEY (work_item_id, project_id) REFERENCES work_items (id, project_id),
    FOREIGN KEY (version_id, project_id) REFERENCES shared_document_versions (id, project_id),
    FOREIGN KEY (run_id, project_id) REFERENCES runs (id, project_id)
);
CREATE INDEX document_pins_version ON document_pins (version_id);

-- Something a work item used has changed: a newer version, a withdrawal.
-- A person checks it; nothing is re-run on its own.
CREATE TABLE impact_checks (
    id           TEXT PRIMARY KEY,
    project_id   TEXT NOT NULL,
    work_item_id TEXT NOT NULL,
    cause        TEXT NOT NULL CHECK (cause IN ('revised', 'withdrawn', 'required_reopened')),
    ref_id       TEXT NOT NULL,
    document_id  TEXT,
    summary      TEXT NOT NULL DEFAULT '',
    status       TEXT NOT NULL DEFAULT 'open' CHECK (status IN ('open', 'resolved', 'dismissed')),
    note         TEXT NOT NULL DEFAULT '',
    created_at   TEXT NOT NULL,
    updated_at   TEXT NOT NULL,
    UNIQUE (work_item_id, cause, ref_id),
    FOREIGN KEY (work_item_id, project_id) REFERENCES work_items (id, project_id)
);
CREATE INDEX impact_checks_open ON impact_checks (project_id, status);

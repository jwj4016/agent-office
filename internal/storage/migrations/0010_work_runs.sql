-- Service operations stage 3: running a work item. Each work item has a
-- small work plan (who, how, what to produce); running it makes a
-- one-step version of a workflow kept only for that item and starts a
-- run linked to the item (docs/plan/service-operations/plan.md §4.6).

-- SQLite cannot add a composite foreign key to an existing table, so the
-- service of these links is checked in code when they are written.
ALTER TABLE workflows ADD COLUMN work_item_id TEXT;
ALTER TABLE runs ADD COLUMN work_item_id TEXT;

-- One hidden workflow per work item, and at most one active run.
CREATE UNIQUE INDEX workflows_work_item ON workflows (work_item_id) WHERE work_item_id IS NOT NULL;
CREATE UNIQUE INDEX runs_work_item_active ON runs (work_item_id)
    WHERE work_item_id IS NOT NULL AND status IN ('running', 'waiting', 'paused');
CREATE INDEX runs_work_item ON runs (work_item_id, started_at);

-- The plan a person writes (or, from stage 3-2, accepts from AI).
-- materials are references the worker should read; outputs use the
-- workflow output format ([{"key": ..., "type": ...}]).
CREATE TABLE work_plans (
    work_item_id  TEXT PRIMARY KEY,
    project_id    TEXT NOT NULL,
    assignment_id TEXT,
    instructions  TEXT NOT NULL DEFAULT '',
    materials     TEXT NOT NULL DEFAULT '',
    outputs       TEXT NOT NULL DEFAULT '[]',
    updated_at    TEXT NOT NULL,
    FOREIGN KEY (work_item_id, project_id) REFERENCES work_items (id, project_id),
    FOREIGN KEY (assignment_id, project_id) REFERENCES assignments (id, project_id)
);

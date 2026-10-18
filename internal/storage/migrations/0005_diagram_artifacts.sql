-- Diagram results (HTML with inline SVG made with the bundled
-- diagram-design skill) are a new artifact type. SQLite cannot change a
-- CHECK constraint in place, so the table is rebuilt with the same
-- columns, keys and index. The new table refers to itself by its new
-- name; the rename then updates that reference, so source_artifact_id
-- stays valid throughout (foreign keys are on during migrations).
CREATE TABLE artifacts_new (
    id              TEXT PRIMARY KEY,
    project_id      TEXT NOT NULL,
    run_id          TEXT NOT NULL,
    step_attempt_id TEXT NOT NULL,
    output_key      TEXT NOT NULL,
    version         INTEGER NOT NULL,
    type            TEXT NOT NULL CHECK (type IN ('markdown', 'json', 'file', 'code_change', 'report', 'diagram')),
    path            TEXT NOT NULL,
    hash            TEXT NOT NULL,
    size            INTEGER NOT NULL,
    validity        TEXT NOT NULL DEFAULT 'valid' CHECK (validity IN ('valid', 'stale', 'invalid')),
    source_artifact_id TEXT REFERENCES artifacts_new (id),
    created_at      TEXT NOT NULL,
    UNIQUE (step_attempt_id, output_key, version),
    UNIQUE (id, project_id),
    FOREIGN KEY (run_id, project_id) REFERENCES runs (id, project_id),
    FOREIGN KEY (step_attempt_id, project_id) REFERENCES step_attempts (id, project_id)
);
-- Foreign keys are checked at the end of each statement, so rows may be
-- copied in any order.
INSERT INTO artifacts_new (id, project_id, run_id, step_attempt_id, output_key, version, type, path, hash, size, validity, source_artifact_id, created_at)
    SELECT id, project_id, run_id, step_attempt_id, output_key, version, type, path, hash, size, validity, source_artifact_id, created_at FROM artifacts;
DROP TABLE artifacts;
ALTER TABLE artifacts_new RENAME TO artifacts;
CREATE INDEX artifacts_run ON artifacts (run_id, output_key);

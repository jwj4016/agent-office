-- Company requests, 6th revision: tool requests the app allows on its own.
-- Safe commands inside a step's folders are allowed by the app's checks
-- (internal/toolpolicy); a person may also let a kind of command run
-- without asking in one service ("npm install"). Risks that are never
-- waived (administrator rights, deleting or writing outside the folders,
-- key files) have no rule and always ask.
CREATE TABLE tool_rules (
    id         TEXT PRIMARY KEY,
    project_id TEXT NOT NULL REFERENCES projects (id),
    program    TEXT NOT NULL CHECK (program <> ''),
    sub        TEXT NOT NULL DEFAULT '',
    created_at TEXT NOT NULL,
    UNIQUE (project_id, program, sub)
);

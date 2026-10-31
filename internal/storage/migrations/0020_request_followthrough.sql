-- Company requests, 5th revision: a request is followed until its work is
-- done (docs/plan/company-requests/plan.md, "다음 단계: 협업 요청의 끝까지
-- 처리").
--
-- goal_plan is the shared goal a desk proposed for work of several
-- departments ({"title", "description", "doneCriteria", "departmentId"});
-- goal_id is the goal made from it (when the first of its work is
-- registered). report is the completion report the desk writes once all
-- the request's work is closed; report_read_at is when the person read it.
ALTER TABLE requests ADD COLUMN goal_plan TEXT DEFAULT NULL;
ALTER TABLE requests ADD COLUMN goal_id TEXT DEFAULT NULL;
ALTER TABLE requests ADD COLUMN report TEXT DEFAULT NULL;
ALTER TABLE requests ADD COLUMN report_read_at TEXT DEFAULT NULL;

-- The work a work item waits for (a JSON list of work item ids of the same
-- service): automatic start holds it until they are all closed.
ALTER TABLE work_items ADD COLUMN after_ids TEXT DEFAULT NULL;

-- One completion-report call. Its usage counts against the service budget.
CREATE TABLE request_reports (
    id             TEXT PRIMARY KEY,
    request_id     TEXT NOT NULL REFERENCES requests (id),
    project_id     TEXT REFERENCES projects (id),
    organization_id TEXT NOT NULL REFERENCES organizations (id),
    status         TEXT NOT NULL DEFAULT 'working' CHECK (status IN ('working', 'succeeded', 'failed', 'interrupted')),
    connection_id  TEXT,
    model          TEXT NOT NULL DEFAULT '',
    usage          TEXT NOT NULL DEFAULT '{}',
    usage_reported INTEGER NOT NULL DEFAULT 0 CHECK (usage_reported IN (0, 1)),
    error          TEXT NOT NULL DEFAULT '',
    started_at     TEXT NOT NULL,
    ended_at       TEXT,
    CHECK (status <> 'failed' OR error <> '')
);
CREATE INDEX request_reports_request ON request_reports (request_id);
CREATE INDEX request_reports_project ON request_reports (project_id);

-- Requests marked done while their registered work is still open are now
-- followed until that work is closed.
UPDATE requests SET status = 'in_progress', closed_at = NULL
WHERE status = 'done' AND EXISTS (
    SELECT 1 FROM request_items i JOIN work_items w ON w.id = i.work_item_id
    WHERE i.request_id = requests.id AND w.status NOT IN ('done', 'cancelled'));

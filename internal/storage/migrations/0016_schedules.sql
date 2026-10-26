-- Service operations stage 8: regular checks. A department can have one
-- schedule (daily or weekly at a time of day, in the time zone of the PC
-- when it was saved). Each due slot becomes one department check event
-- that goes through the same judgment as a person's check request
-- (docs/plan/service-operations/plan.md §4.3, §9.1).

-- One schedule per department: it is changed or turned off, never
-- replaced, so its slots keep counting. Slots due before active_from
-- (the last save) are not run, so saving never fires a past slot.
CREATE TABLE department_schedules (
    id            TEXT PRIMARY KEY,
    project_id    TEXT NOT NULL,
    department_id TEXT NOT NULL UNIQUE,
    frequency     TEXT NOT NULL CHECK (frequency IN ('daily', 'weekly')),
    time_of_day   TEXT NOT NULL CHECK (length(time_of_day) = 5 AND time_of_day GLOB '[0-2][0-9]:[0-5][0-9]' AND time_of_day < '24:00'),
    weekday       INTEGER CHECK (weekday BETWEEN 0 AND 6), -- 0 is Sunday
    timezone      TEXT NOT NULL CHECK (timezone <> ''),
    note          TEXT NOT NULL DEFAULT '',  -- what the department looks for
    enabled       INTEGER NOT NULL DEFAULT 1 CHECK (enabled IN (0, 1)),
    active_from   TEXT NOT NULL,
    created_at    TEXT NOT NULL,
    updated_at    TEXT NOT NULL,
    UNIQUE (id, project_id),
    FOREIGN KEY (department_id, project_id) REFERENCES departments (id, project_id),
    CHECK ((frequency = 'weekly') = (weekday IS NOT NULL))
);

-- A slot is one due time of a schedule ("daily:2026-10-27",
-- "weekly:2026-W44"). Its key is unique, so a slot runs once however the
-- schedule is changed, turned off and on, or retried.
CREATE TABLE schedule_slots (
    schedule_id    TEXT NOT NULL,
    project_id     TEXT NOT NULL,
    slot_key       TEXT NOT NULL,
    due_at         TEXT NOT NULL,
    event_sequence INTEGER NOT NULL REFERENCES execution_events (sequence),
    created_at     TEXT NOT NULL,
    PRIMARY KEY (schedule_id, slot_key),
    FOREIGN KEY (schedule_id, project_id) REFERENCES department_schedules (id, project_id)
);

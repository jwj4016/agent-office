-- M4: Git worktrees per step attempt (spec §10.1).
--
-- kind: base   = the run's repository and base commit (one per run)
--       shared = no Git: every step works in this folder (one per run)
--       code   = a step's own worktree on its own branch
--       view   = a read-only checkout for steps that do not change code
-- path is the worktree root; subdir is where the service lives inside it.
ALTER TABLE workspaces ADD COLUMN kind TEXT NOT NULL DEFAULT 'code';
ALTER TABLE workspaces ADD COLUMN repo TEXT NOT NULL DEFAULT '';
ALTER TABLE workspaces ADD COLUMN subdir TEXT NOT NULL DEFAULT '';
ALTER TABLE workspaces ADD COLUMN start_commit TEXT NOT NULL DEFAULT '';
ALTER TABLE workspaces ADD COLUMN commit_sha TEXT NOT NULL DEFAULT '';
-- commits still to merge after the first conflict is resolved
ALTER TABLE workspaces ADD COLUMN pending_merges TEXT NOT NULL DEFAULT '[]';
ALTER TABLE workspaces ADD COLUMN conflicts TEXT NOT NULL DEFAULT '[]';
ALTER TABLE workspaces ADD COLUMN note TEXT NOT NULL DEFAULT '';
CREATE INDEX workspaces_run ON workspaces (run_id);

-- Preserve the draft that was reviewed, and the proposal a follow-up refines.
ALTER TABLE workflow_assists ADD COLUMN base_workflow TEXT NOT NULL DEFAULT '{}';
ALTER TABLE workflow_assists ADD COLUMN parent_id TEXT REFERENCES workflow_assists (id);
ALTER TABLE workflow_assists ADD COLUMN scope_node_id TEXT NOT NULL DEFAULT '';

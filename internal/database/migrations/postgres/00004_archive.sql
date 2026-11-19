-- Archiving closed backlog items (FR-155). An archived item is hidden from
-- the default table views, the board and the sprint planning selection, but
-- stays part of the product state. NULL means not archived; an archived item
-- is always in a closed status.
ALTER TABLE requirements ADD COLUMN archived_at TIMESTAMPTZ;
ALTER TABLE defects ADD COLUMN archived_at TIMESTAMPTZ;
CREATE INDEX IF NOT EXISTS idx_requirements_archived ON requirements(product_id, archived_at);
CREATE INDEX IF NOT EXISTS idx_defects_archived ON defects(product_id, archived_at);

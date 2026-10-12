-- Existing direct-save entries are all maintenance; future writers name their object type.
ALTER TABLE ops.audit_log ADD COLUMN target_type text NOT NULL DEFAULT 'MAINTENANCE';
ALTER TABLE ops.audit_log ALTER COLUMN target_type DROP DEFAULT;

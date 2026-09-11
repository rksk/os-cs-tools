DROP INDEX IF EXISTS idx_case_attachments_status_created;
ALTER TABLE case_attachments DROP COLUMN IF EXISTS status;

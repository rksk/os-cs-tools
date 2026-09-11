BEGIN;

-- case_attachments stores metadata only. File bytes for this data source live
-- externally in SFTPGo, addressed by storage_key -- there is no base64-payload
-- alternative here, unlike the ServiceNow data source's /attachments API.
CREATE TABLE IF NOT EXISTS case_attachments (
  id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  case_id     UUID NOT NULL REFERENCES cases(id),
  storage_key TEXT NOT NULL,
  filename    TEXT NOT NULL,
  mime_type   TEXT NOT NULL,
  size_bytes  BIGINT NOT NULL CHECK (size_bytes > 0),
  description TEXT,
  uploaded_by TEXT NOT NULL REFERENCES users(id),
  created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_by  TEXT REFERENCES users(id)
);

-- FK / equality indexes
CREATE INDEX IF NOT EXISTS idx_case_attachments_case_id     ON case_attachments(case_id);
CREATE INDEX IF NOT EXISTS idx_case_attachments_uploaded_by ON case_attachments(uploaded_by);

-- Composite index for the paginated per-case feed (most recent first).
CREATE INDEX IF NOT EXISTS idx_case_attachments_case_created ON case_attachments(case_id, created_at DESC);

COMMIT;

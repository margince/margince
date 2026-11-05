SET LOCAL lock_timeout = '3s';

DROP INDEX IF EXISTS idx_lead_origin;
CREATE INDEX idx_lead_origin ON lead (origin)
    WHERE archived_at IS NULL AND origin IS NOT NULL;

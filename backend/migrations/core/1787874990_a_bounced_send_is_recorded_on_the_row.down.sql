-- Dropping these returns a bounced send to being indistinguishable from one
-- that arrived, which is what it was before.
SET LOCAL lock_timeout = '3s';

DROP INDEX IF EXISTS comms_outbound_bounced_idx;

ALTER TABLE comms_outbound DROP CONSTRAINT IF EXISTS comms_outbound_bounce_kind_named;
ALTER TABLE comms_outbound DROP CONSTRAINT IF EXISTS comms_outbound_bounce_is_stated;

ALTER TABLE comms_outbound
    DROP COLUMN IF EXISTS bounced_at,
    DROP COLUMN IF EXISTS bounce_kind,
    DROP COLUMN IF EXISTS bounce_reason;

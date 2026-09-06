-- Bound the wait for the locks below. Without it an open transaction holding a
-- conflicting lock stalls every write to consent_event for as long as this
-- migration is willing to queue, which is forever.
SET LOCAL lock_timeout = '3s';

-- Restoring NOT NULL needs every NULL filled, and the only value available is
-- the placeholder this migration exists to remove. Rows written after it are
-- withdrawals that genuinely showed nothing, so the down path names them for
-- what they are rather than claiming wording they never had.
ALTER TABLE consent_event DROP CONSTRAINT IF EXISTS consent_event_wording_pairs;

UPDATE consent_event SET policy_text = 'no wording recorded' WHERE policy_text IS NULL;
UPDATE consent_event SET policy_version = 'v1' WHERE policy_version IS NULL;

ALTER TABLE consent_event ALTER COLUMN policy_text SET NOT NULL;
ALTER TABLE consent_event ALTER COLUMN policy_version SET NOT NULL;

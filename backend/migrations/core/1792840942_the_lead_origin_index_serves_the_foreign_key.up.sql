-- Deleting a lead_origin checks lead.origin through an index. The partial index
-- from the first origin migration cannot serve that check, because its
-- predicate names archived_at, which the check does not.
SET LOCAL lock_timeout = '3s';

DROP INDEX idx_lead_origin;
CREATE INDEX idx_lead_origin ON lead (origin);

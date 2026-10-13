-- pgmigrate:no-transaction
-- The follow-up queue's sales arm finds a working lead by id, by the contact it
-- came from, or by the contact it was promoted to (activities customerArms). An
-- OR is read from indexes only when every arm has one, so without this index
-- each candidate message read the whole lead table.
--
-- Built concurrently, so lead writes are never blocked for the build. The drop
-- ahead of the build clears an invalid index a failed build left behind.
DROP INDEX CONCURRENTLY IF EXISTS idx_lead_promoted_contact;
CREATE INDEX CONCURRENTLY idx_lead_promoted_contact ON lead (promoted_contact_id) WHERE promoted_contact_id IS NOT NULL;

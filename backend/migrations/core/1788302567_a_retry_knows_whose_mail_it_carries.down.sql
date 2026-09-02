-- Bounded, because every statement below takes a lock that blocks writers on a
-- table the deliverer writes on every event. Without a ceiling, one open
-- transaction holding a conflicting lock stalls every webhook write for as long
-- as this migration is willing to queue.
SET LOCAL lock_timeout = '3s';

ALTER TABLE webhook_delivery
	DROP CONSTRAINT webhook_delivery_status_check;

-- Any row parked as visibility_revoked has no pre-existing status that means
-- the same thing. dead_lettered is the closest: both are terminal and neither
-- is retried without a human asking.
--
-- dead_lettered_at is set alongside it, because the downgraded code reads that
-- column as "when did this stop" and a NULL there on a terminal row is a shape
-- recordOutcome never writes. attempts is left as it stands: the row genuinely
-- was not attempted that many times, and inflating it to the budget would make
-- the history claim six failures that did not happen.
--
-- The reason is lost, and these rows become replayable again by code that does
-- not re-check. That is the honest cost of a downgrade past this migration and
-- is why it is a development path, not an operational one.
UPDATE webhook_delivery
   SET status = 'dead_lettered',
       dead_lettered_at = coalesce(dead_lettered_at, now())
 WHERE status = 'visibility_revoked';

ALTER TABLE webhook_delivery
	ADD CONSTRAINT webhook_delivery_status_check
	CHECK (status IN ('pending', 'delivered', 'retrying', 'dead_lettered'));

ALTER TABLE webhook_delivery
	DROP COLUMN entity_id,
	DROP COLUMN entity_type;

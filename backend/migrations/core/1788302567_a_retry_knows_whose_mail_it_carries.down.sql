-- Bounded, because every statement below takes a lock that blocks writers on a
-- table the deliverer writes on every event. Without a ceiling, one open
-- transaction holding a conflicting lock stalls every webhook write for as long
-- as this migration is willing to queue.
SET LOCAL lock_timeout = '3s';

ALTER TABLE webhook_delivery
	DROP CONSTRAINT webhook_delivery_status_check;

-- Any row parked as visibility_revoked has no pre-existing status that means
-- the same thing. dead_lettered is the closest: both are terminal and neither
-- will be retried without a human asking.
UPDATE webhook_delivery SET status = 'dead_lettered' WHERE status = 'visibility_revoked';

ALTER TABLE webhook_delivery
	ADD CONSTRAINT webhook_delivery_status_check
	CHECK (status IN ('pending', 'delivered', 'retrying', 'dead_lettered'));

ALTER TABLE webhook_delivery
	DROP COLUMN entity_id,
	DROP COLUMN entity_type;

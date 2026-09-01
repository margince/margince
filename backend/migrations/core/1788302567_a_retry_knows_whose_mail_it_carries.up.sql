-- A parked delivery records WHICH record it carries and WHOSE subscription
-- asked for it, so a retry can ask the visibility question again.
--
-- The enqueue gate is correct and stays: ownerCanSee resolves the owner's live
-- RBAC and tests the subject entity against it. What was missing is that a
-- delivery, once parked, was re-attempted from a frozen payload with no second
-- look — so narrowing an activity after enqueue did not stop the retry, and a
-- replay (operator-triggered, attempt budget reset) shipped it too.
--
-- Recovering the subject by parsing the stored payload was the alternative and
-- is not safe: bodies written before the current wire mapping do not carry the
-- entity ref in a stable place, and a parse failure would force a choice
-- between parking those rows forever and re-opening the hole. Columns say it
-- once, at the moment the answer is known.

-- Bounded, because every statement below takes a lock that blocks writers on a
-- table the deliverer writes on every event. Without a ceiling, one open
-- transaction holding a conflicting lock stalls every webhook write for as long
-- as this migration is willing to queue.
SET LOCAL lock_timeout = '3s';

ALTER TABLE webhook_delivery
	ADD COLUMN entity_type text,
	ADD COLUMN entity_id uuid;

COMMENT ON COLUMN webhook_delivery.entity_type IS
	'The subject record this delivery carries, written at enqueue. NULL on rows '
	'that predate this column, and NULL means unknown means the retry refuses: a '
	'row whose subject cannot be identified cannot be re-checked against its '
	'audience, and shipping it would be exactly the disclosure this closes.';

-- A fifth status, rather than reusing dead_lettered. Two reasons, both about
-- what an operator reads afterwards: dead_lettered is the store they replay
-- from, and a revoked delivery must not be in it; and resetForReplay clears
-- last_error, so recording the reason there would destroy it on the next
-- replay attempt.
ALTER TABLE webhook_delivery
	DROP CONSTRAINT webhook_delivery_status_check;

ALTER TABLE webhook_delivery
	ADD CONSTRAINT webhook_delivery_status_check
	CHECK (status IN ('pending', 'delivered', 'retrying', 'dead_lettered', 'visibility_revoked'));

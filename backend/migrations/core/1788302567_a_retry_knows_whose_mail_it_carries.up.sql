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

-- NOT VALID, then VALIDATE. Adding a validated CHECK scans every existing row
-- while holding ACCESS EXCLUSIVE, and lock_timeout bounds only the WAIT to
-- acquire that lock, never how long the scan then holds it — on a mature
-- webhook_delivery that blocks every delivery write for the length of a full
-- scan. NOT VALID takes the lock only long enough to record the constraint;
-- VALIDATE re-reads the table under a SHARE UPDATE EXCLUSIVE that writers do
-- not queue behind.
--
-- The new constraint is strictly wider than the one it replaces, so no existing
-- row can violate it and the validation cannot fail. It is still run rather
-- than skipped: a NOT VALID constraint is not enforced for pre-existing rows on
-- later reads, and leaving it unvalidated would mean the table's stated
-- vocabulary and its enforced one differ.
ALTER TABLE webhook_delivery
	ADD CONSTRAINT webhook_delivery_status_check
	CHECK (status IN ('pending', 'delivered', 'retrying', 'dead_lettered', 'visibility_revoked'))
	NOT VALID;

ALTER TABLE webhook_delivery
	VALIDATE CONSTRAINT webhook_delivery_status_check;

-- Every delivery still in flight when this lands has no recorded subject, and
-- nothing can give it one: the stored payload does not carry the entity ref
-- (mapping.go builds the body from the event's own fields, not its envelope).
-- So the retry path will refuse them, one at a time, silently, as each comes
-- due — a subscriber whose endpoint was down during the deploy would simply
-- stop receiving, with the reason spread across however many rows over however
-- many hours.
--
-- Saying it once, here, is the honest version: the rows are parked now, with a
-- reason an operator can read and a single query that finds all of them. They
-- are not lost — the events they carry are still in the bus's own record, and a
-- subscriber who needs them asks for a re-send rather than replaying a delivery
-- nobody can authorize.
UPDATE webhook_delivery
   SET status = 'visibility_revoked',
       next_retry_at = NULL,
       last_error = 'parked by the subject-columns migration: this delivery was '
                    'enqueued before webhook_delivery recorded which record it '
                    'carries, so its visibility cannot be re-checked'
 WHERE status IN ('pending', 'retrying')
   AND entity_type IS NULL;

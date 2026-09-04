-- Return the kind vocabulary to the four the up migration found.
--
-- The derived rows go first, and they are DELETED rather than converted. A
-- `meeting` row is a derivation, not a human's assertion: it says only "this
-- meeting is on the record", and the meeting itself survives this rollback, so
-- nothing is lost that the forward migration cannot derive again on the next
-- send. Converting them to `in_person` would be worse than losing them — that
-- kind means a human stated an exchange happened, and inventing the note the
-- CHECK demands would put words in their mouth.
--
-- The evidence CHECK is restored first so the table is never briefly held to a
-- rule the surviving rows fail.

-- The table is small and these are catalog-only rewrites, but the ALTERs still take
-- ACCESS EXCLUSIVE: an open transaction holding a conflicting lock would stall every
-- write to the table for as long as this is willing to queue. Bounded, so a busy
-- database fails the deploy instead of freezing consent writes.
SET LOCAL lock_timeout = '3s';

DELETE FROM consent_qualifying_event WHERE kind = 'meeting';

ALTER TABLE consent_qualifying_event
  DROP CONSTRAINT consent_qualifying_event_evidence;

ALTER TABLE consent_qualifying_event
  ADD CONSTRAINT consent_qualifying_event_evidence
  CHECK (
    (kind = 'in_person'::text AND note IS NOT NULL)
    OR (kind <> 'in_person'::text
        AND source_entity_type IS NOT NULL
        AND source_entity_id IS NOT NULL)
  );

ALTER TABLE consent_qualifying_event
  DROP CONSTRAINT consent_qualifying_event_kind_check;

ALTER TABLE consent_qualifying_event
  ADD CONSTRAINT consent_qualifying_event_kind_check
  CHECK (kind = ANY (ARRAY['inbound_message'::text, 'inquiry'::text,
                           'active_deal'::text, 'in_person'::text]));

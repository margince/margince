-- Narrowing the kind back.
--
-- THE ROWS ARE CONVERTED, NOT DELETED, which is the precedent the meeting
-- rollback set for exactly this case. A rep recorded that somebody rang and
-- asked for a quote, correspondence went out on that ground, and deleting the
-- row would destroy the evidence for a send that has already happened — leaving
-- the record unable to answer why it was lawful, and refusing the next message
-- to the same person for want of a fact that was true and is now gone.
--
-- They become `in_person`, which is the other hand-recorded kind and has the
-- same shape: a note, no source row. The word is then wrong about HOW the
-- exchange happened, so the note says so — a reader who finds "in person" with
-- a sentence explaining it was a request preserved through a rollback can tell
-- what actually occurred, which a silently relabelled row could not.
--
-- Nothing can collide: the unique index over
-- (person_id, source_entity_type, source_entity_id) is partial on
-- source_entity_id IS NOT NULL, and these rows have none.
SET LOCAL lock_timeout = '3s';

UPDATE consent_qualifying_event
   SET kind = 'in_person',
       note = coalesce(note, '') ||
              ' (Recorded as a request the person made themselves, preserved when that kind was ' ||
              'rolled back.)'
 WHERE kind = 'requested_by_subject';

ALTER TABLE consent_qualifying_event
  DROP CONSTRAINT consent_qualifying_event_evidence;

ALTER TABLE consent_qualifying_event
  ADD CONSTRAINT consent_qualifying_event_evidence
  CHECK (
    (kind = 'in_person'::text AND note IS NOT NULL)
    OR (kind <> 'in_person'::text
        AND source_entity_type IS NOT NULL
        AND source_entity_id IS NOT NULL));

ALTER TABLE consent_qualifying_event
  DROP CONSTRAINT consent_qualifying_event_kind_check;

ALTER TABLE consent_qualifying_event
  ADD CONSTRAINT consent_qualifying_event_kind_check
  CHECK (kind = ANY (ARRAY['inbound_message'::text, 'inquiry'::text,
                           'active_deal'::text, 'in_person'::text,
                           'meeting'::text]));

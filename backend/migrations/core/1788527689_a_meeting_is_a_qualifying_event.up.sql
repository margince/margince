-- A meeting the subject attended is a qualifying event.
--
-- Ordinary business correspondence is lawful under Art 6(1)(f) when something on
-- the record connects us to the person. The engine derived that from ONE thing:
-- an inbound message they wrote. A meeting is stronger evidence than an email —
-- an email can be unsolicited, while a meeting means both sides put time in a
-- calendar — and the engine could not see one, so a partner we were meeting next
-- week was refused as somebody who "has never written to you".
--
-- `meeting` rather than reusing `in_person`, which the evidence CHECK below
-- already treats as the hand-recorded kind: it demands a note and carries no
-- source row. A derived event is the opposite shape — it needs
-- source_entity_id, because that is what the unique index makes idempotent, so
-- re-deriving the same meeting on the next send cannot stack a second row
-- claiming a second exchange happened. Reusing the kind would have meant one
-- word describing two shapes, and the CHECK would have refused the derived one
-- outright.
--
-- Both CHECKs are dropped and re-added because a CHECK cannot be altered in
-- place. The rewrite is additive in effect: every value either constraint
-- admitted before is still admitted.

ALTER TABLE consent_qualifying_event
  DROP CONSTRAINT consent_qualifying_event_kind_check;

ALTER TABLE consent_qualifying_event
  ADD CONSTRAINT consent_qualifying_event_kind_check
  CHECK (kind = ANY (ARRAY['inbound_message'::text, 'inquiry'::text,
                           'active_deal'::text, 'in_person'::text,
                           'meeting'::text]));

-- The evidence rule is unchanged in substance: the hand-recorded kind needs its
-- note, and every derived kind needs the source row it was read from. `meeting`
-- falls in the second group with no clause of its own, which is the point —
-- a kind that needed an exception here would be a kind nothing could evidence.
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

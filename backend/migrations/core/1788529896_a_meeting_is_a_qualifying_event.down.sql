-- Return the kind vocabulary to the four the up migration found.
--
-- The `meeting` rows are CONVERTED, never deleted. Each one is the Art 5(2)
-- record of what made an already-sent message lawful — the controller carries
-- the burden of showing its basis, and a rollback that destroys the proof leaves
-- sends that happened with nothing standing behind them. Re-deriving later does
-- not repair that: it would record today's reading of the record, not the fact
-- as it stood when the message went out.
--
-- They become `in_person`, which is the kind whose evidence is a human-readable
-- note rather than a source row, and the note says plainly what the row is so
-- nobody later mistakes it for something a person asserted. The source columns
-- are cleared because the restored evidence CHECK forbids them on that kind.
--
-- The unique index over (person_id, source_entity_type, source_entity_id) is
-- partial on source_entity_id IS NOT NULL, so clearing those columns drops these
-- rows out of it and no conversion can collide.

SET LOCAL lock_timeout = '3s';

UPDATE consent_qualifying_event
   SET kind = 'in_person',
       note = coalesce(note, '') ||
              'Derived from a meeting on ' ||
              to_char(occurred_at, 'YYYY-MM-DD') ||
              ' (activity ' || coalesce(source_entity_id::text, 'unknown') ||
              '), preserved when the meeting kind was rolled back.',
       source_entity_type = NULL,
       source_entity_id = NULL
 WHERE kind = 'meeting';

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

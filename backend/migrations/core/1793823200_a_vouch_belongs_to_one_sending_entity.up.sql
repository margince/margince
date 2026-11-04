-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

-- A standing vouch is shared by the reps of one sending legal entity and never
-- reaches another entity's mail. Each row names the entity whose rep recorded
-- it.
--
-- A row recorded before this column existed takes the entity of the seat that
-- recorded the first row of its merge chain: a human by its user, an agent by
-- the user its passport works for. That user's entity is their one
-- legal_entity_member row, else the default entity. A row whose recorder this
-- cannot name is revoked rather than guessed, so a rep who still needs it
-- records it again.
SET LOCAL lock_timeout = '3s';

ALTER TABLE communication_override ADD COLUMN legal_entity_id uuid;

WITH RECURSIVE chain AS (
    SELECT id, id AS root, carried_from, captured_by FROM communication_override
  UNION ALL
    SELECT chain.id, parent.id, parent.carried_from, parent.captured_by
      FROM chain JOIN communication_override parent ON parent.id = chain.carried_from
), recorder AS (
    SELECT chain.id,
           coalesce(human.id, passport.on_behalf_of) AS user_id
      FROM chain
      LEFT JOIN app_user human ON chain.captured_by = 'human:' || human.id::text
      LEFT JOIN passport ON chain.captured_by = 'agent:' || passport.id::text
     WHERE chain.carried_from IS NULL
)
UPDATE communication_override o
   SET legal_entity_id = coalesce(
       (SELECT le.id FROM legal_entity_member m
          JOIN legal_entity le ON le.id = m.legal_entity_id
         WHERE m.user_id = recorder.user_id),
       (SELECT le.id FROM legal_entity le WHERE le.is_default))
  FROM recorder
 WHERE recorder.id = o.id AND recorder.user_id IS NOT NULL;

UPDATE communication_override
   SET legal_entity_id = (SELECT le.id FROM legal_entity le WHERE le.is_default),
       revoked_at = coalesce(revoked_at, now())
 WHERE legal_entity_id IS NULL;

ALTER TABLE communication_override
    ALTER COLUMN legal_entity_id SET NOT NULL,
    ADD CONSTRAINT communication_override_legal_entity_id_fkey
        FOREIGN KEY (legal_entity_id) REFERENCES legal_entity(id) ON DELETE RESTRICT;

DROP INDEX communication_override_live_contact;
CREATE INDEX communication_override_live_contact
    ON communication_override (contact_id, legal_entity_id, category) WHERE revoked_at IS NULL;
CREATE INDEX communication_override_legal_entity
    ON communication_override (legal_entity_id);

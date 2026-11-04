-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

-- A standing vouch is shared by the reps of one sending legal entity and never
-- reaches another entity's mail. Each row names the entity whose rep recorded
-- it. A row recorded before this column existed takes the entity of the user
-- who recorded it, else the default entity.
SET LOCAL lock_timeout = '3s';

ALTER TABLE communication_override ADD COLUMN legal_entity_id uuid;

UPDATE communication_override o
   SET legal_entity_id = coalesce(
       (SELECT m.legal_entity_id FROM legal_entity_member m
         WHERE o.captured_by = 'user:' || m.user_id::text),
       (SELECT le.id FROM legal_entity le WHERE le.is_default));

ALTER TABLE communication_override
    ALTER COLUMN legal_entity_id SET NOT NULL,
    ADD CONSTRAINT communication_override_legal_entity_id_fkey
        FOREIGN KEY (legal_entity_id) REFERENCES legal_entity(id) ON DELETE RESTRICT;

DROP INDEX communication_override_live_contact;
CREATE INDEX communication_override_live_contact
    ON communication_override (contact_id, legal_entity_id, category) WHERE revoked_at IS NULL;
CREATE INDEX communication_override_legal_entity
    ON communication_override (legal_entity_id);

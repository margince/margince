-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

SET LOCAL lock_timeout = '5s';

-- A refused send is no longer handed to a colleague as an approval card, so a
-- review never waits on one. The state that said so, the column naming the card
-- and the index serving the deciders' queue go.

-- No review should still read 'awaiting_decision': the migration that retired
-- the ask moved them all back to the rep. Any left would break the narrowed
-- CHECK below, so they move the same way, with one audit entry each.
WITH returned AS (
    UPDATE communication_review
       SET state = 'needs_context'
     WHERE state = 'awaiting_decision'
    RETURNING id
)
INSERT INTO audit_log (actor_type, actor_id, action, entity_type, entity_id, before, after)
SELECT 'system', 'migration', 'update', 'communication_review', id,
       '{"state":"awaiting_decision"}'::jsonb, '{"state":"needs_context"}'::jsonb
  FROM returned;

DROP INDEX IF EXISTS communication_review_awaiting;

-- NOT VALID, because this table holds history: the scan runs in the next
-- migration, under a lock readers and writers pass.
ALTER TABLE communication_review
    DROP CONSTRAINT communication_review_routing_shape,
    DROP CONSTRAINT communication_review_state,
    ADD CONSTRAINT communication_review_state CHECK (state = ANY (ARRAY[
        'needs_context'::text, 'needs_repair'::text,
        'resolved'::text, 'superseded'::text, 'cancelled'::text])) NOT VALID;

ALTER TABLE communication_review DROP COLUMN approval_id;

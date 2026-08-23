-- Sharing a document with a buyer is sharing it. The room no longer asks a
-- buyer to formally confirm each file: what they want to say about a document
-- they say in the thread under it.
--
-- The `reviewer` seat existed only to carry that confirmation, so it collapses
-- into `comment`, which is what it already granted. Existing reviewers keep
-- exactly the authority they have been exercising.
SET LOCAL lock_timeout = '5s';

UPDATE deal_room_participant SET capability = 'comment' WHERE capability = 'reviewer';

ALTER TABLE deal_room_participant
    DROP CONSTRAINT deal_room_participant_capability_check;

ALTER TABLE deal_room_participant
    ADD CONSTRAINT deal_room_participant_capability_check
        CHECK (capability IN ('view', 'comment'));

DROP TABLE IF EXISTS deal_room_decision;

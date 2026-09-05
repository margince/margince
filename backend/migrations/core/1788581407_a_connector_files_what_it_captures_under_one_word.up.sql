-- A connector files what it captures under one word.
--
-- An import already does this: a person picks an existing tag at the start of a
-- run, and every record the run creates is filed under it, so a batch stays
-- findable as a batch. A record capture creates — a contact recognised from a
-- mailbox — carried no equivalent, so "which records came in from this source"
-- had no answer.
--
-- The CONNECTOR is the batch. A mailbox polls forever, so a tag per sync window
-- would mint a new word per window that nobody chose, growing without bound and
-- filling a human-curated vocabulary with machine timestamps. The connector is
-- the discrete thing an operator set up, and it is what "this source" means.
-- The period half of the question is answered by the record's own created_at,
-- which already exists.
--
-- Nullable, and null is the honest default: a connector whose operator chose no
-- word files nothing, rather than being guessed a word for.
--
-- ON DELETE SET NULL rather than RESTRICT: a tag is a shared vocabulary entry
-- and deleting one is the vocabulary's decision, not this connector's veto. A
-- connector then files nothing, which is the same state as never having chosen
-- — and the alternative, a foreign key that refuses the delete, would make one
-- mailbox's setting able to block a workspace-wide edit.
SET LOCAL lock_timeout = '3s';

ALTER TABLE capture_connection
    ADD COLUMN context_tag_id uuid
        REFERENCES tag (id) ON DELETE SET NULL;

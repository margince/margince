SET LOCAL lock_timeout = '3s';

DROP INDEX IF EXISTS communication_review_awaiting;

-- The rows first: a review still reading 'awaiting_decision' would violate the
-- narrowed CHECK below, and a rollback that cannot apply is worse than the
-- change it is undoing. They go back to needing context, which is where they
-- were before anybody routed them.
UPDATE communication_review
   SET state = 'needs_context'
 WHERE state = 'awaiting_decision';

ALTER TABLE communication_review
    DROP CONSTRAINT IF EXISTS communication_review_routing_shape,
    DROP CONSTRAINT IF EXISTS communication_review_state,
    ADD CONSTRAINT communication_review_state CHECK (state = ANY (ARRAY[
        'needs_context'::text, 'needs_repair'::text,
        'resolved'::text, 'superseded'::text, 'cancelled'::text])),
    DROP COLUMN IF EXISTS approval_id;

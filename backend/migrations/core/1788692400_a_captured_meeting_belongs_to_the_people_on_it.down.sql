-- Reverting "a captured meeting belongs to the people on it".
--
-- BEST EFFORT, and the overlap constraint is why. Step 1 of the up migration
-- stamped a host onto every captured meeting, and restoring the original
-- predicate re-admits those rows to the double-booking guard — where a seat's
-- genuine back-to-back appointments collide and the ALTER fails outright.
--
-- Check before running this:
--
--   SELECT count(*) FROM activity a JOIN activity b
--     ON a.host_user_id = b.host_user_id AND a.id <> b.id
--    AND tsrange(timezone('UTC', a.occurred_at), timezone('UTC', a.occurred_at) + interval '1 hour')
--     && tsrange(timezone('UTC', b.occurred_at), timezone('UTC', b.occurred_at) + interval '1 hour')
--    WHERE a.kind = 'meeting' AND a.archived_at IS NULL AND b.archived_at IS NULL;
--
-- A nonzero answer means this file cannot run as written, and the fix is
-- forward: the released code must keep the narrowed constraint. Nothing here
-- clears host_user_id to make the restore succeed — that would throw away which
-- seat's calendar each meeting came from, and the up migration cannot recover it
-- a second time for a row whose import history has since been pruned.

SET LOCAL lock_timeout = '5s';

DROP TABLE IF EXISTS activity_meeting_attendee_repair;

-- The audience and the person visibility are NOT reverted. Both are privacy
-- narrowings, and widening them again would republish a seat's private
-- appointments and an agent's unreviewed contacts to the whole workspace — a
-- disclosure that cannot be undone by re-running the up migration. A deployment
-- that genuinely wants the old behaviour turns it off in code, where the
-- decision is visible, rather than having a rollback publish records nobody
-- asked to publish.

ALTER TABLE activity
    DROP CONSTRAINT activity_meeting_no_overlap;

ALTER TABLE activity
    ADD CONSTRAINT activity_meeting_no_overlap
    EXCLUDE USING gist (
        host_user_id WITH =,
        tsrange(timezone('UTC'::text, occurred_at),
                (timezone('UTC'::text, occurred_at) + '01:00:00'::interval)) WITH &&)
    WHERE (kind = 'meeting'::text
           AND host_user_id IS NOT NULL
           AND archived_at IS NULL);

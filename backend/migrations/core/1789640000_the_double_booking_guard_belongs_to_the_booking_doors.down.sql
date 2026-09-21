-- Bounded: this rewrites a constraint on a table every write touches, so a
-- transaction already holding a conflicting lock must not be able to stall them
-- for as long as this is willing to queue.
SET LOCAL lock_timeout = '3s';

ALTER TABLE activity
    DROP CONSTRAINT activity_meeting_no_overlap;

ALTER TABLE activity
    DROP COLUMN claims_host_slot;

ALTER TABLE activity
    ADD CONSTRAINT activity_meeting_no_overlap
    EXCLUDE USING gist (
        host_user_id WITH =,
        tsrange(timezone('UTC'::text, occurred_at),
                (timezone('UTC'::text, occurred_at) + '01:00:00'::interval)) WITH &&)
    WHERE (kind = 'meeting'::text
           AND host_user_id IS NOT NULL
           AND archived_at IS NULL
           AND source_system IS NULL);

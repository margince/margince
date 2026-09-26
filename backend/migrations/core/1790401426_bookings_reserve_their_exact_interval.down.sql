SET LOCAL lock_timeout = '3s';
ALTER TABLE activity DROP CONSTRAINT activity_exact_booking_duration;
ALTER TABLE activity DROP CONSTRAINT activity_meeting_no_overlap;
ALTER TABLE activity ADD CONSTRAINT activity_meeting_no_overlap
    EXCLUDE USING gist (host_user_id WITH =,
        tsrange(timezone('UTC', occurred_at), timezone('UTC', occurred_at) + interval '1 hour') WITH &&)
    WHERE (kind = 'meeting' AND host_user_id IS NOT NULL AND archived_at IS NULL AND claims_host_slot);
ALTER TABLE activity DROP COLUMN booking_interval_exact;

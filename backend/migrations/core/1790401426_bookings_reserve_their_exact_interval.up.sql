SET LOCAL lock_timeout = '3s';

-- Legacy bookings without a recorded duration retain their historical hour.
-- Exact lengths already stored on old rows cannot safely enlarge reservations
-- during rollout; those rows keep the old range until explicitly rescheduled.
ALTER TABLE activity ADD COLUMN booking_interval_exact boolean NOT NULL DEFAULT false;
ALTER TABLE activity DROP CONSTRAINT activity_meeting_no_overlap;
ALTER TABLE activity ADD CONSTRAINT activity_meeting_no_overlap
    EXCLUDE USING gist (
        host_user_id WITH =,
        tsrange(timezone('UTC', occurred_at), timezone('UTC', occurred_at) +
            CASE WHEN booking_interval_exact THEN duration_seconds ELSE 3600 END * interval '1 second', '[)') WITH &&)
    WHERE (kind = 'meeting' AND host_user_id IS NOT NULL AND archived_at IS NULL
        AND claims_host_slot AND meeting_status IS DISTINCT FROM 'canceled');

ALTER TABLE activity ADD CONSTRAINT activity_exact_booking_duration CHECK (NOT booking_interval_exact OR (duration_seconds IS NOT NULL AND duration_seconds > 0));

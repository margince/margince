-- Dropping the marker table only. The meeting statuses the pass wrote stay:
-- they are the correct answer about those meetings, and re-opening a cancelled
-- meeting as booked would put it back on somebody's schedule.

SET LOCAL lock_timeout = '3s';

DROP TABLE IF EXISTS activity_meeting_rsvp_backfill;

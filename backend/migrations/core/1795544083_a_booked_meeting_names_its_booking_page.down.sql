SET LOCAL lock_timeout = '3s';

DROP INDEX IF EXISTS meeting_invitation_booking_page_idx;
ALTER TABLE meeting_invitation DROP COLUMN IF EXISTS booking_page_id;

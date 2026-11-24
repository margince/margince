-- 1795544083: a_booked_meeting_names_its_booking_page.
SET LOCAL lock_timeout = '3s';

-- The booking page a public booking came through: the host's personal page or
-- one of their team links. A move reads the calendars of that page's confirmed
-- members alone. NULL on every invitation booked before the column, and on one
-- no booking page took.
ALTER TABLE meeting_invitation ADD COLUMN IF NOT EXISTS booking_page_id uuid
    REFERENCES booking_page(id) ON DELETE SET NULL;
CREATE INDEX IF NOT EXISTS meeting_invitation_booking_page_idx ON meeting_invitation (booking_page_id);

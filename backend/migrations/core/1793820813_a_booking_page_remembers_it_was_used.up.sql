-- A booking page records the first time it was used: somebody booked through
-- it, or its host copied its link. Set once and never cleared. Home warns a
-- host about a paused page only once the page has been used.
SET LOCAL lock_timeout = '3s';

ALTER TABLE booking_page ADD COLUMN first_used_at timestamptz;

-- A live page somebody already booked through counts as used from its first
-- public booking. booking_page holds one live row per host.
UPDATE booking_page AS page
   SET first_used_at = booked.first_at
  FROM (SELECT host_user_id, min(created_at) AS first_at
          FROM meeting_invitation
         WHERE public_intent IS NOT NULL
         GROUP BY host_user_id) AS booked
 WHERE page.host_user_id = booked.host_user_id
   AND page.revoked_at IS NULL;

-- 1795540594: a_host_keeps_team_links_beside_the_personal_link.
SET LOCAL lock_timeout = '3s';

-- A host has one personal booking page and any number of team links. A team
-- link names the colleagues who attend each meeting booked through it; the
-- personal page offers its host's own free time alone.
ALTER TABLE booking_page ADD COLUMN IF NOT EXISTS team_link boolean NOT NULL DEFAULT false;

-- Every writer kept one live page per host, so this revokes nothing on a
-- sound database; it lets the index below be built on any database.
UPDATE booking_page AS page SET revoked_at = now()
 WHERE page.revoked_at IS NULL
   AND EXISTS (SELECT 1 FROM booking_page newer
                WHERE newer.host_user_id = page.host_user_id AND newer.revoked_at IS NULL
                  AND (newer.created_at, newer.id) > (page.created_at, page.id));

CREATE UNIQUE INDEX IF NOT EXISTS booking_page_one_personal_per_host ON booking_page (host_user_id)
 WHERE revoked_at IS NULL AND NOT team_link;

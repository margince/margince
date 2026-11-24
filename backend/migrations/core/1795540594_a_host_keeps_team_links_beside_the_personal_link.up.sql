-- 1795540594: a_host_keeps_team_links_beside_the_personal_link.
SET LOCAL lock_timeout = '3s';

-- A host has one personal booking page and any number of team links. A team
-- link names the colleagues who attend each meeting booked through it; the
-- personal page offers its host's own free time alone.
ALTER TABLE booking_page ADD COLUMN IF NOT EXISTS team_link boolean NOT NULL DEFAULT false;

-- Every writer kept one live page per host, so this revokes nothing on a
-- sound database; it lets the index below be built on any database. Of a
-- host's live pages the one in use stays: the one its live alias names, then
-- one that was used (first_used_at is stamped by a booking or a copied link),
-- then the newest.
UPDATE booking_page AS page SET revoked_at = now()
  FROM (SELECT p.id, row_number() OVER (PARTITION BY p.host_user_id ORDER BY
               EXISTS (SELECT 1 FROM booking_alias a WHERE a.booking_page_id = p.id AND a.retired_at IS NULL) DESC,
               p.first_used_at IS NOT NULL DESC, p.created_at DESC, p.id DESC) AS rank
          FROM booking_page p WHERE p.revoked_at IS NULL) AS ranked
 WHERE page.id = ranked.id AND ranked.rank > 1;

CREATE UNIQUE INDEX IF NOT EXISTS booking_page_one_personal_per_host ON booking_page (host_user_id)
 WHERE revoked_at IS NULL AND NOT team_link;

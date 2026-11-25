SET LOCAL lock_timeout = '3s';

-- Without the column a team link would read as a second personal page.
UPDATE booking_page SET revoked_at = now() WHERE team_link AND revoked_at IS NULL;
DROP INDEX IF EXISTS booking_page_one_personal_per_host;
ALTER TABLE booking_page DROP COLUMN IF EXISTS team_link;

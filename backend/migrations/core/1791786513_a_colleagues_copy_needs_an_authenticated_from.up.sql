-- A colleague's copied mail is ours only when Gmail authenticated its From
-- (capture/sentbycolleague.go fromAuthenticated). Migration 1791775182 re-read
-- copies on the From address alone; this checks each against its stored
-- original and turns back to inbound every copy that fails.
--
-- The SQL reads the original the way mailmap.TopSenderAuthentication does: the
-- topmost Authentication-Results header, unfolded, quoted strings and then
-- comments removed, signed mx.google.com. It must hold no quote or parenthesis
-- left over, exactly one dmarc clause, and that clause a pass for the From domain. A copy captured by any
-- other transport, or whose original is gone, cannot show that and turns back.
SET LOCAL lock_timeout = '3s';

WITH result AS (
  SELECT a.id, u.email AS colleague,
         regexp_replace(regexp_replace(
           substring(regexp_replace(substring(rc.payload #>> '{}' FROM '^(.*?)\r?\n\r?\n'), '\r?\n[ \t]+', ' ', 'g')
                     FROM '(?in)^authentication-results:(.*)$'),
           '"[^"]*"', ' ', 'g'), '\([^()]*\)', ' ', 'g') AS line
    FROM activity a
    JOIN app_user u ON u.id = a.sent_by_colleague
    LEFT JOIN raw_capture rc ON rc.id = a.raw_capture_id
   WHERE a.sent_by_colleague IS NOT NULL
), unproven AS (
  SELECT r.id, r.colleague FROM result r JOIN activity a ON a.id = r.id
   WHERE a.captured_by NOT LIKE 'connector:gmail:%'
      OR r.line IS NULL
      OR r.line ~ '["()]'
      OR regexp_count(r.line, ';\s*dmarc=', 1, 'i') <> 1
      OR lower(btrim(split_part(r.line, ';', 1))) <> 'mx.google.com'
      OR lower(substring(r.line FROM '(?i);\s*dmarc=pass\M[^;]*header\.from=([^[:space:];]+)'))
         IS DISTINCT FROM lower(split_part(r.colleague, '@', 2))
)
UPDATE activity a
   SET direction = 'inbound', counterparty_email = lower(u.colleague), sent_by_colleague = NULL
  FROM unproven u
 WHERE a.id = u.id;

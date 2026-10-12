-- A colleague's copied mail is ours only when Gmail authenticated its From
-- (capture/sentbycolleague.go fromAuthenticated). Migration 1791775182 re-read
-- copies on the From address alone; this checks each against its stored
-- original and turns back to inbound every copy that fails. A copy the
-- colleague's own mailbox has attested since is proven another way and stays.
--
-- The SQL reads the original the way mailmap.TopSenderAuthentication does: the
-- topmost Authentication-Results header, unfolded, quoted strings and then
-- comments removed, under the authserv-id mx.google.com. No quote or
-- parenthesis may be left over. It must hold exactly one dmarc clause in any
-- spelling, and that clause must be a pass under DMARC version 1 or no version,
-- whose header.from is the From domain. A base64-wrapped original is decoded
-- first, as rawCapturePayload stores one.
--
-- What a turned-back copy made true goes with it: the colleague as its sender,
-- a model's settlement verdict judged through it, and the reminder that verdict
-- completed when no human touched it.
SET LOCAL lock_timeout = '3s';

WITH original AS (
  SELECT a.id, a.captured_by, u.email AS colleague, a.sent_by_colleague,
         CASE WHEN jsonb_typeof(rc.payload) = 'string' THEN rc.payload #>> '{}'
              WHEN rc.payload ->> 'encoding' = 'base64'
               AND (SELECT count(*) FROM jsonb_object_keys(rc.payload)) = 2 AND rc.payload ? 'data'
              THEN encode(decode(rc.payload ->> 'data', 'base64'), 'escape')
         END AS raw
    FROM activity a
    JOIN app_user u ON u.id = a.sent_by_colleague
    LEFT JOIN raw_capture rc ON rc.id = a.raw_capture_id
   WHERE a.sent_by_colleague IS NOT NULL AND NOT a.counterparty_outbound_attested
), result AS (
  SELECT o.*,
         regexp_replace(regexp_replace(
           substring(regexp_replace(substring(o.raw FROM '^(.*?)\r?\n\r?\n'), '\r?\n[ \t]+', ' ', 'g')
                     FROM '(?in)^authentication-results:(.*)$'),
           '"[^"]*"', ' ', 'g'), '\([^()]*\)', ' ', 'g') AS line
    FROM original o
), unproven AS (
  SELECT r.id, r.colleague, r.sent_by_colleague FROM result r
   WHERE r.captured_by NOT LIKE 'connector:gmail:%'
      OR r.line IS NULL
      OR r.line ~ '["()]'
      OR lower(split_part(btrim(split_part(r.line, ';', 1)), ' ', 1)) <> 'mx.google.com'
      OR regexp_count(r.line, ';\s*dmarc\s*(/[^=;]*)?=', 1, 'i') <> 1
      OR lower(substring(r.line FROM '(?i);\s*dmarc\s*(?:/\s*1\s*)?=\s*pass\s+(?:[^;]*\s)?header\.from=([^[:space:];]+)'))
         IS DISTINCT FROM lower(split_part(r.colleague, '@', 2))
), reverted AS (
  UPDATE activity a
     SET direction = 'inbound', counterparty_email = lower(u.colleague), sent_by_colleague = NULL
    FROM unproven u
   WHERE a.id = u.id
  RETURNING a.id, u.sent_by_colleague AS colleague
), unsent AS (
  DELETE FROM activity_participant p USING reverted
   WHERE p.activity_id = reverted.id AND p.role = 'from' AND p.user_id = reverted.colleague
), unjudged AS (
  DELETE FROM activity_request_settlement s USING reverted
   WHERE s.judged_through_activity_id = reverted.id
  RETURNING s.request_activity_id, s.verdict
)
UPDATE activity r
   SET is_done = false, done_at = NULL
  FROM unjudged
 WHERE unjudged.verdict = 'settled'
   AND r.source_system = 'email_request' AND r.source_activity_id = unjudged.request_activity_id
   AND r.is_done AND r.archived_at IS NULL
   AND NOT EXISTS (SELECT 1 FROM audit_log worked
         WHERE worked.entity_type = 'activity' AND worked.entity_id = r.id AND worked.actor_type = 'human');

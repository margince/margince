-- A colleague's copied mail that migration 1791775182 could not see is re-read
-- as the workspace's own outbound mail, as capture now reads it.
--
-- 1791775182 found the colleague through the stored `from` participant. Capture
-- stamps no such row when the capturing seat claims the colleague's address or
-- its domain as their own (stampCaptureParticipants), so that mail stayed
-- inbound. An inbound row's counterparty_email is its From, which is what
-- capture/sentbycolleague.go reads, so the colleague is found there instead.
--
-- Each re-read row gets an audit row naming this migration, which is how the
-- down migration finds it. What the owed pass left on these rows while they read
-- as questions goes, as 1791778974 removed it from the rows 1791775182 re-read.
--
-- Idempotent: a re-read row is outbound, so a second run selects nothing.
SET LOCAL lock_timeout = '3s';

WITH own_domain AS (
  SELECT lower(domain) AS domain FROM workspace_email_domain
   UNION
  SELECT lower(d.domain) FROM company_domain d JOIN company o ON o.id = d.company_id
   WHERE o.is_anchor AND o.archived_at IS NULL AND d.archived_at IS NULL
), colleague_mail AS (
  SELECT a.id, u.id AS colleague,
         (SELECT lower(btrim(r.address)) FROM activity_participant r
           WHERE r.activity_id = a.id AND r.role IN ('to', 'cc', 'bcc')
             AND coalesce(btrim(r.address), '') <> ''
             AND lower(btrim(r.address)) <> lower(u.email)
             AND NOT EXISTS (SELECT 1 FROM own_domain o
                   WHERE split_part(lower(btrim(r.address)), '@', 2) = o.domain
                      OR split_part(lower(btrim(r.address)), '@', 2) LIKE '%.' || o.domain)
             AND NOT EXISTS (SELECT 1 FROM capture_owner_identity i
                   WHERE i.user_id::text = split_part(a.captured_by, ':', 3)
                     AND ((i.kind = 'address' AND lower(i.value) = lower(btrim(r.address)))
                       OR (i.kind = 'domain' AND (lower(i.value) = split_part(lower(btrim(r.address)), '@', 2)
                         OR split_part(lower(btrim(r.address)), '@', 2) LIKE '%.' || lower(i.value)))))
             AND NOT EXISTS (SELECT 1 FROM capture_connection cc
                   WHERE cc.user_id::text = split_part(a.captured_by, ':', 3) AND cc.archived_at IS NULL
                     AND lower(btrim(coalesce(substring(cc.account_label from '<([^>]+)>'), cc.account_label)))
                         = lower(btrim(r.address)))
             AND NOT EXISTS (SELECT 1 FROM app_user seat
                   WHERE seat.id::text = split_part(a.captured_by, ':', 3)
                     AND lower(seat.email) = lower(btrim(r.address)))
           ORDER BY array_position(ARRAY['to', 'cc', 'bcc'], r.role), r.id
           LIMIT 1) AS recipient
    FROM activity a
    -- A seat created after the mail was captured was not a colleague when
    -- capture read it: a customer invited later keeps their questions.
    JOIN app_user u ON lower(u.email) = a.counterparty_email AND u.archived_at IS NULL
     AND u.created_at <= a.created_at
   WHERE a.kind = 'email' AND a.direction = 'inbound' AND NOT a.has_calendar_part
     AND a.captured_by LIKE 'connector:%'
     AND split_part(a.captured_by, ':', 3) <> u.id::text
), reread AS (
UPDATE activity a
   SET direction = 'outbound',
       sent_by_colleague = c.colleague,
       counterparty_email = c.recipient,
       owed_verdict = NULL, owed_verdict_at = NULL, owed_verdict_ruleset = NULL,
       owed_verdict_declined_at = NULL, owed_verdict_declined_ruleset = NULL,
       reply_verdict = NULL, reply_verdict_at = NULL, reply_verdict_by = NULL
  FROM colleague_mail c
 WHERE a.id = c.id AND c.recipient IS NOT NULL
RETURNING a.id, c.colleague
), stamped AS (
INSERT INTO activity_participant (activity_id, role, user_id)
SELECT id, 'from', colleague FROM reread
ON CONFLICT DO NOTHING
)
INSERT INTO audit_log (actor_type, actor_id, action, entity_type, entity_id, after)
SELECT 'system', 'migration', 'update', 'activity', id,
       jsonb_build_object('migration', '1791782100_a_colleagues_mail_is_ours_under_an_own_address')
  FROM reread;

-- Every colleague copy, as in 1791778974: the earlier ones were cleaned there,
-- and capture files no request on outbound mail, so these are the rows above.
DELETE FROM activity_request_settlement s USING activity a
 WHERE s.request_activity_id = a.id AND a.sent_by_colleague IS NOT NULL;

-- The reminders are locked first, and the human test runs in the next
-- statement, for the reason 1791778974 gives.
SELECT count(*) FROM (
  SELECT r.id FROM activity r JOIN activity a ON a.id = r.source_activity_id
   WHERE a.sent_by_colleague IS NOT NULL
     AND r.source_system = 'email_request' AND NOT r.is_done AND r.archived_at IS NULL
     FOR UPDATE OF r
) locked;

WITH retired AS (
  UPDATE activity r
     SET archived_at = now()
    FROM activity a
   WHERE a.sent_by_colleague IS NOT NULL
     AND r.source_system = 'email_request' AND r.source_activity_id = a.id
     AND NOT r.is_done AND r.archived_at IS NULL
     AND NOT EXISTS (SELECT 1 FROM audit_log worked
           WHERE worked.entity_type = 'activity' AND worked.entity_id = r.id AND worked.actor_type = 'human')
  RETURNING r.id
)
INSERT INTO audit_log (actor_type, actor_id, action, entity_type, entity_id, after)
SELECT 'system', 'migration', 'archive', 'activity', id,
       jsonb_build_object('migration', '1791782100_a_colleagues_mail_is_ours_under_an_own_address')
  FROM retired;

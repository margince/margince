-- A colleague's mail to an outside party is the workspace's own outbound mail.
--
-- A connected mailbox also receives copies of mail its colleagues send: Marcus
-- answers a customer and copies Lars. Capture filed that copy as INBOUND, since
-- its From is not the mailbox owner, so the customer's question stayed owed and
-- the colleague's answer became a question owed to the colleague.
--
-- sent_by_colleague names the seat whose address is in From. It is not an
-- attestation: no provider vouched for this copy, so counterparty_outbound_attested
-- stays false and no correspondence gate trusts the row. The settlement and
-- answer reads accept "attested OR sent by a colleague" in one conversation.
SET LOCAL lock_timeout = '3s';
ALTER TABLE activity ADD COLUMN sent_by_colleague uuid
  REFERENCES app_user(id) ON DELETE SET NULL;
-- NOT VALID: the column is NULL on every existing row, so the scan would prove
-- nothing, and the backfill below writes only rows that satisfy it.
ALTER TABLE activity ADD CONSTRAINT activity_colleague_mail_is_outbound
  CHECK (sent_by_colleague IS NULL OR direction = 'outbound') NOT VALID;

-- The rows captured before this rule, re-read the way capture now reads them
-- (capture/sentbycolleague.go): an email whose From is the exact login address
-- of an unarchived seat, of any status, other than the mailbox that captured it. The counterparty is
-- the first recipient, To before Cc before Bcc, that is not the colleague, not
-- at an own domain or a subdomain of one, and not one of the capturing seat's
-- own addresses. A mail with no such recipient stays as it is.
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
                       OR (i.kind = 'domain' AND lower(i.value) = split_part(lower(btrim(r.address)), '@', 2))))
             AND NOT EXISTS (SELECT 1 FROM app_user seat
                   WHERE seat.id::text = split_part(a.captured_by, ':', 3)
                     AND lower(seat.email) = lower(btrim(r.address)))
           ORDER BY array_position(ARRAY['to', 'cc', 'bcc'], r.role), r.id
           LIMIT 1) AS recipient
    FROM activity a
    JOIN activity_participant f ON f.activity_id = a.id AND f.role = 'from' AND f.user_id IS NULL
    JOIN app_user u ON lower(u.email) = lower(btrim(f.address))
     AND u.archived_at IS NULL
   WHERE a.kind = 'email' AND a.direction = 'inbound' AND NOT a.has_calendar_part
     AND a.captured_by LIKE 'connector:%'
     AND split_part(a.captured_by, ':', 3) <> u.id::text
)
UPDATE activity a
   SET direction = 'outbound',
       sent_by_colleague = c.colleague,
       counterparty_email = c.recipient,
       owed_verdict = NULL, owed_verdict_at = NULL, owed_verdict_ruleset = NULL,
       owed_verdict_declined_at = NULL, owed_verdict_declined_ruleset = NULL,
       reply_verdict = NULL, reply_verdict_at = NULL, reply_verdict_by = NULL
  FROM colleague_mail c
 WHERE a.id = c.id AND c.recipient IS NOT NULL;

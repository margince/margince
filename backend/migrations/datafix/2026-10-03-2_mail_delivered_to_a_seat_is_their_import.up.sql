SET LOCAL lock_timeout = '3s';
-- Capture recorded a seat's import of a message only when one of the seat's
-- exact addresses stood on its From, To, Cc or Bcc line. Mail that reached the
-- mailbox through a list, a group address or a Bcc names none of them there,
-- so it was stored with no capture_import row: no arrival time, no verdict, and
-- invisible to every rule that asks what the mailbox already held. Capture now
-- also counts the Delivered-To the receiving server wrote
-- (capture.mailboxWasARecipientTx).
--
-- This writes the missing row where the stored original proves the same thing:
--   * the activity's captured_by names the seat's own connection, and that seat
--     has no import row for it yet;
--   * the FIRST Delivered-To or Received line of the original's head is a
--     Delivered-To — a receiving server prepends its Received above whatever a
--     sender wrote, so only a Delivered-To above every Received is the
--     server's own (mailmap.TopDeliveredTo);
--   * that address is an exact address the seat holds: one in
--     capture_owner_identity, or the account a live connection of theirs was
--     granted for.
-- An import row is a vote in the activity's audience (activities.contributionOf),
-- so each row carries what keeps the message exactly as private as it is now:
-- for a held message, the seat's mailbox posture and the hold's own reason, and
-- `pending` where it waits on a verdict. A row that would still read as a vote
-- for the workspace on a held message is not written at all. Run the data fix
-- 1790871111_mail_a_mailbox_already_held_owes_no_notice afterwards; it stamps
-- the arrival time on these rows and settles the notice duties they excuse.
WITH candidate AS (
    SELECT a.id, u.id AS seat, a.audience, a.audience_reason, split_part(a.captured_by, ':', 2) AS provider,
           CASE
             WHEN jsonb_typeof(rc.payload) = 'string' THEN left(rc.payload #>> '{}', 65536)
             WHEN left(rc.payload ->> 'data', 87380) ~ '^([A-Za-z0-9+/]{4})*([A-Za-z0-9+/]{2}==|[A-Za-z0-9+/]{3}=)?$'
             THEN encode(decode(left(rc.payload ->> 'data', 87380), 'base64'), 'escape')
           END AS text_head
      FROM activity a
      JOIN raw_capture rc ON rc.id = a.raw_capture_id
      JOIN app_user u ON u.id::text = split_part(a.captured_by, ':', 3)
     WHERE a.kind = 'email' AND a.restricted_at IS NULL
       AND a.captured_by ~ '^connector:[a-z0-9_-]+:[0-9a-f-]{36}$'
       AND NOT EXISTS (SELECT 1 FROM capture_import ci WHERE ci.activity_id = a.id AND ci.user_id = u.id)
),
first_line AS (
    SELECT id, seat, audience, audience_reason, provider,
           regexp_match(E'\n' || substring(text_head FROM '^(.*?)\r?\n\r?\n'),
                        '\n(delivered-to|received):[ \t]*([^\r\n]*)', 'i') AS m
      FROM candidate
),
delivered AS (
    SELECT id, seat, audience, audience_reason, provider,
           lower(btrim(coalesce(substring(m[2] FROM '<([^>]*)>'), m[2]))) AS address
      FROM first_line
     WHERE lower(m[1]) = 'delivered-to'
),
held_as AS (
    SELECT d.id, d.seat, d.audience, d.address,
           CASE WHEN d.audience = 'participants' THEN
             (SELECT cc.mail_posture FROM capture_connection cc
               WHERE cc.user_id = d.seat AND cc.provider = d.provider AND cc.archived_at IS NULL
               ORDER BY cc.created_at DESC LIMIT 1)
           END AS posture,
           CASE WHEN d.audience = 'participants' AND d.audience_reason = 'pending_verdict' THEN 'pending' END AS status,
           CASE WHEN d.audience = 'participants' THEN d.audience_reason END AS reason
      FROM delivered d
)
INSERT INTO capture_import (activity_id, user_id, posture_at_import, verdict_status, verdict_reason)
SELECT h.id, h.seat, h.posture, h.status, h.reason
  FROM held_as h
 WHERE h.address <> ''
   AND (h.audience <> 'participants' OR h.status = 'pending' OR h.posture IN ('held', 'classified'))
   AND (EXISTS (SELECT 1 FROM capture_owner_identity oi
                 WHERE oi.user_id = h.seat AND oi.kind = 'address' AND oi.value = h.address)
        OR EXISTS (SELECT 1 FROM capture_connection cc
                    WHERE cc.user_id = h.seat AND cc.archived_at IS NULL
                      AND lower(btrim(coalesce(substring(cc.account_label FROM '<([^>]*)>'), cc.account_label)))
                          = h.address))
ON CONFLICT (activity_id, user_id) DO NOTHING;

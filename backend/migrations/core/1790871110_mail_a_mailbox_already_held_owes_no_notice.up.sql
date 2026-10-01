SET LOCAL lock_timeout = '3s';
-- When the mailbox PROVIDER says a message arrived in this seat's mailbox:
-- Gmail's internalDate, Graph's receivedDateTime. Capture fills it from the
-- provider's API response, not from the message, so a sender cannot backdate
-- it the way they can write any Date header. NULL when the transport stated
-- none, and NULL proves nothing.
ALTER TABLE capture_import ADD COLUMN IF NOT EXISTS provider_received_at timestamptz;

-- Mail captured before this column existed has no API time on record. The
-- nearest thing the provider wrote is the TOPMOST Received header of the stored
-- original: a receiving server prepends its Received line, so the first one in
-- the header block is the provider's own delivery stamp, in a position the
-- sender cannot write (the argument mailmap.TopDeliveredTo makes for
-- Delivered-To). Only inbound mail, and only where exactly one seat imported
-- the message, because the stored original is the copy the FIRST mailbox
-- delivered and its stamp says nothing about a second seat's copy. A stamp that
-- does not parse cleanly as an RFC 5322 date stays NULL.
WITH held AS (
    SELECT ci.id,
           substring(CASE
                       WHEN jsonb_typeof(rc.payload) = 'string' THEN left(rc.payload #>> '{}', 262144)
                       -- Bytes jsonb would not take as text travel base64
                       -- (capture.rawCapturePayload). The header block is
                       -- ASCII; the escape encoding leaves CR and LF as they
                       -- are and spells only the other bytes out.
                       ELSE encode(substring(decode(rc.payload ->> 'data', 'base64') FROM 1 FOR 262144), 'escape')
                     END FROM '^(.*?)\r?\n\r?\n') AS head
      FROM capture_import ci
      JOIN activity a ON a.id = ci.activity_id
      JOIN raw_capture rc ON rc.id = a.raw_capture_id
     WHERE ci.provider_received_at IS NULL
       AND a.kind = 'email' AND a.direction = 'inbound'
       AND (jsonb_typeof(rc.payload) = 'string'
            OR (jsonb_typeof(rc.payload) = 'object' AND rc.payload ->> 'encoding' = 'base64'
                AND jsonb_typeof(rc.payload -> 'data') = 'string'))
       AND NOT EXISTS (SELECT 1 FROM capture_import o
                        WHERE o.activity_id = ci.activity_id AND o.user_id <> ci.user_id)
),
top_received AS (
    SELECT id, (regexp_match(head, '(?:^|\n)received:((?:[^\n]|\n[ \t])*)', 'i'))[1] AS line
      FROM held
),
stamp AS (
    SELECT id, regexp_match(
             btrim(regexp_replace(regexp_replace((regexp_match(line, ';([^;]*)$'))[1],
                                                 '\([^)]*\)', '', 'g'), '\s+', ' ', 'g')),
             '^(?:[A-Za-z]{3}, ?)?(\d{1,2}) ([A-Za-z]{3}) (\d{4}) (\d{2}):(\d{2})(?::(\d{2}))? ([+-]\d{4})$') AS m
      FROM top_received
),
parts AS (
    SELECT id, m[7] AS zone, m[1]::int AS d,
           array_position(ARRAY['jan', 'feb', 'mar', 'apr', 'may', 'jun',
                                'jul', 'aug', 'sep', 'oct', 'nov', 'dec'], lower(m[2])) AS mo,
           m[3]::int AS y, m[4]::int AS hh, m[5]::int AS mi, coalesce(m[6], '00')::int AS ss
      FROM stamp
     WHERE m IS NOT NULL
),
stamped AS (
    SELECT id, format('%s-%s-%s %s:%s:%s%s', y, mo, d, hh, mi, ss, zone)::timestamptz AS at
      FROM parts
     WHERE mo IS NOT NULL AND y BETWEEN 1970 AND 9999 AND hh <= 23 AND mi <= 59 AND ss <= 59
       AND d BETWEEN 1 AND extract(day FROM make_date(y, mo, 1) + interval '1 month - 1 day')::int
)
UPDATE capture_import ci
   SET provider_received_at = stamped.at
  FROM stamped
 WHERE ci.id = stamped.id;

-- Capture's own unknowns are reclassified as mailbox_history when the company's
-- mailbox already held the correspondence before the seat connected it, by
-- either rule capture now applies (spelled again here because a migration
-- cannot call Go):
--   * contacts.writtenToBeforeConnectedTx: a mail the provider filed as sent to
--     one of the contact's addresses, sent by the seat whose mailbox delivered
--     it, and dated before that seat's first connection;
--   * contacts.receivedBeforeConnectedTx: a received, non-bulk mail naming one
--     of the contact's addresses on To or Cc, which the provider says arrived in
--     the importing seat's mailbox before that seat's first connection.
-- "Capture's own" means a connector or the counterparty verdict, which creates
-- contacts from captured mail after the fact. Migration 1790844232 looked at
-- connector:* only and so left every verdict-made contact owing a notice. An
-- unknown a seat or an import stated is a claim about somewhere else and stays.
--
-- Their open duties close as exempt_with_reason with the ground on the row and
-- one audit entry each, as 1790844232 does. The cases are locked before they
-- are read, so the audit before-image is the row that was overwritten; a queued
-- case keeps its delivery, and a terminal one keeps its ground.
WITH history AS (
    UPDATE contact_acquisition_evidence e
       SET kind = 'mailbox_history'
     WHERE e.kind = 'unknown_legacy'
       AND (starts_with(e.captured_by, 'connector:') OR e.captured_by = 'agent:capture_counterparty_verdict')
       AND (EXISTS (
              SELECT 1 FROM contact_email ce
                JOIN activity a ON a.counterparty_email = ce.email
                JOIN capture_import ci ON ci.activity_id = a.id
                JOIN activity_participant s
                  ON s.activity_id = a.id AND s.role = 'from' AND s.user_id = ci.user_id
               WHERE ce.contact_id = e.contact_id AND ce.archived_at IS NULL
                 AND a.kind = 'email' AND a.direction = 'outbound'
                 AND a.counterparty_outbound_attested AND a.archived_at IS NULL
                 AND a.occurred_at < (SELECT min(cc.created_at) FROM capture_connection cc
                                       WHERE cc.user_id = ci.user_id))
         OR EXISTS (
              SELECT 1 FROM contact_email ce
                JOIN activity_participant p ON p.address = ce.email AND p.role IN ('to', 'cc')
                JOIN activity a ON a.id = p.activity_id
                JOIN capture_import ci ON ci.activity_id = a.id
               WHERE ce.contact_id = e.contact_id AND ce.archived_at IS NULL
                 AND a.kind = 'email' AND a.direction = 'inbound'
                 AND NOT a.bulk_mail_attested AND a.archived_at IS NULL
                 AND ci.provider_received_at < (SELECT min(cc.created_at) FROM capture_connection cc
                                                 WHERE cc.user_id = ci.user_id)))
    RETURNING e.id
),
owed AS (
    SELECT c.id, c.state, c.rule, c.owner_user_id
      FROM privacy_notice_case c
     WHERE c.acquisition_id IN (SELECT id FROM history)
       AND c.state IN ('open', 'assigned', 'blocked', 'delivery_failed')
       FOR UPDATE
),
settled AS (
    UPDATE privacy_notice_case c
       SET state = 'exempt_with_reason',
           resolution_note = 'Mailbox history: before a company mailbox was connected, it already held mail we sent them or mail we received that was also addressed to them, so the correspondence was already held and no new notice is owed.',
           completed_at = now(), blocked_reason = NULL, updated_at = now()
      FROM owed
     WHERE c.id = owed.id
       AND c.state IN ('open', 'assigned', 'blocked', 'delivery_failed')
    RETURNING c.id
)
INSERT INTO audit_log (actor_type, actor_id, action, entity_type, entity_id, before, after)
SELECT 'system', 'migration', 'update', 'privacy_notice_case', owed.id,
       jsonb_strip_nulls(jsonb_build_object('state', owed.state, 'owner_user_id', owed.owner_user_id)),
       jsonb_build_object('state', 'exempt_with_reason', 'rule', owed.rule, 'resolution_note', true)
  FROM owed JOIN settled ON settled.id = owed.id;

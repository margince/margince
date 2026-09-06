-- A captured meeting names whose calendar it came from.
--
-- Two facts are repaired here, both of which the capture path now writes for
-- new rows. Existing rows were written before it did, and a code fix alone
-- leaves them exactly as exposed as they were.
--
-- 1. host_user_id, recovered from captured_by. The connector stamps
--    'connector:gcal:<uuid>' and an agent 'agent:<uuid>', so the seat that
--    captured the row is already recorded — it was simply never copied into
--    the column the read path needs. Only rows whose recovered id is a real
--    app_user are touched; anything else stays NULL rather than pointing the
--    column at an id nothing backs.
--
-- 2. audience, for the meetings that link no record. A link-less meeting was
--    born 'workspace', which ActivityDiscoverClause reads as a workspace-shared
--    note. Those rows are re-derived to 'participants' with the reason the
--    capture path would now stamp.
--
-- Meetings only, and only rows that still carry the default. A meeting somebody
-- deliberately widened, or one already held for a stronger reason (a
-- counterparty hold, a confidentiality marker), keeps the answer it has: this
-- repairs rows nothing ever decided about, and must not overwrite a decision.

UPDATE activity a
   SET host_user_id = u.id
  FROM app_user u
 WHERE a.kind = 'meeting'
   AND a.host_user_id IS NULL
   AND a.captured_by ~ '^(connector:[a-z0-9_-]+|agent):[0-9a-fA-F-]{36}$'
   AND u.id = substring(a.captured_by from '([0-9a-fA-F-]{36})$')::uuid;

UPDATE activity a
   SET audience = 'participants',
       audience_reason = 'no_counterparty'
 WHERE a.kind = 'meeting'
   AND a.audience = 'workspace'
   AND a.audience_reason IS NULL
   AND a.counterparty_email IS NULL
   AND NOT EXISTS (SELECT 1 FROM activity_link l WHERE l.activity_id = a.id);

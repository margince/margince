-- Home attention inventory: what an installation's history left in the
-- Worklist, counted and read-only. docs/how-to/inventory-home-attention.md
-- says how to run it and how to read each count.
--
-- The transaction is READ ONLY and ends in ROLLBACK, so no statement here can
-- change a row, even after an edit. Counts only: no subject, body or address
-- leaves the database.

\set ON_ERROR_STOP on
\if :{?horizon_days}
\else
\set horizon_days 90
\endif

-- The machine-sender markers of transactional.go, separators allowed where a
-- sender writes no-reply or do-not-reply apart.
\set machine_words 'no[._-]?repl(y|ies)|do[._-]?not[._-]?reply|not[._-]?reply|nreply|notifications?|notify|mailer[._-]?daemon|auto[._-]?reply|automailer|automated|receipts?'

BEGIN TRANSACTION READ ONLY;

\echo '1. Received mail by origin'
-- Mail a seat's mailbox already held when it was first connected is imported
-- history.
WITH first_connection AS (
  SELECT user_id, min(created_at) AS connected_at
    FROM capture_connection
   GROUP BY user_id)
SELECT CASE WHEN ci.provider_received_at IS NULL THEN 'arrival time not recorded'
            WHEN fc.connected_at IS NULL THEN 'no mailbox connection on record'
            WHEN ci.provider_received_at < fc.connected_at THEN 'held before the mailbox was connected'
            ELSE 'arrived after connection' END AS origin,
       count(DISTINCT a.id) AS messages
  FROM activity a
  JOIN capture_import ci ON ci.activity_id = a.id
  LEFT JOIN first_connection fc ON fc.user_id = ci.user_id
 WHERE a.kind = 'email' AND a.direction = 'inbound' AND a.archived_at IS NULL
 GROUP BY 1 ORDER BY 1;

\echo '2. Open confirmed requests by age against the horizon, and whether a human holds them'
-- Open and confirmed as backend/internal/modules/activities/requeststate.go
-- spells them: no finished reminder, no standing settlement, not judged not
-- sales, and either classified as asking us or taken by a human.
SELECT a.occurred_at >= now() - make_interval(days => :horizon_days) AS within_horizon,
       EXISTS (SELECT 1 FROM activity held
                WHERE held.source_system = 'email_request' AND held.source_activity_id = a.id
                  AND NOT held.is_done AND held.archived_at IS NULL
                  AND EXISTS (SELECT 1 FROM audit_log worked
                               WHERE worked.entity_type = 'activity' AND worked.entity_id = held.id
                                 AND worked.actor_type = 'human')) AS held_by_a_human,
       count(*) AS requests
  FROM activity a
 WHERE a.kind IN ('email', 'message') AND a.direction = 'inbound'
   AND a.archived_at IS NULL AND a.restricted_at IS NULL
   AND NOT EXISTS (SELECT 1 FROM activity settled
                    WHERE settled.source_system = 'email_request'
                      AND settled.source_activity_id = a.id AND settled.is_done)
   AND NOT EXISTS (SELECT 1 FROM activity_request_settlement answered
                    WHERE answered.request_activity_id = a.id AND answered.verdict = 'settled'
                      AND NOT EXISTS (SELECT 1 FROM activity reopened
                                       WHERE reopened.source_system = 'email_request'
                                         AND reopened.source_activity_id = a.id
                                         AND NOT reopened.is_done AND reopened.archived_at IS NULL))
   AND NOT EXISTS (SELECT 1 FROM activity_sales_state dismissed
                    WHERE dismissed.thread_key = a.thread_key AND dismissed.kind = a.kind
                      AND dismissed.channel_provider = coalesce(a.channel_provider, ''))
   AND (a.owed_verdict = 'asks_us'
     OR EXISTS (SELECT 1 FROM activity accepted
                 WHERE accepted.source_system = 'email_request' AND accepted.source_activity_id = a.id))
 GROUP BY 1, 2 ORDER BY 1, 2;

\echo '3. Received mail whose sender is a seat, a connected mailbox, or a claimed address'
SELECT count(*) AS suspect_direction
  FROM activity a
 WHERE a.kind = 'email' AND a.direction = 'inbound' AND a.archived_at IS NULL
   AND (lower(a.counterparty_email) IN (SELECT lower(email) FROM app_user)
     OR lower(a.counterparty_email) IN (SELECT lower(account_label) FROM capture_connection
                                         WHERE archived_at IS NULL)
     OR lower(a.counterparty_email) IN (SELECT lower(value) FROM capture_owner_identity
                                         WHERE kind = 'address'));

\echo '4. Unresolved privacy-notice duties: state, and whether recorded after their deadline'
SELECT state,
       created_at > due_at + interval '1 day' AS recorded_after_deadline,
       count(*) AS duties
  FROM privacy_notice_case
 WHERE state NOT IN ('completed', 'not_required', 'exempt_with_reason', 'provided_elsewhere')
 GROUP BY 1, 2 ORDER BY 1, 2;

\echo '5. Pending approvals by kind'
-- Pending as the decision queue reads it: not past its expiry, except a held
-- scheduled send, which waits without one.
SELECT kind, count(*) AS pending
  FROM approval
 WHERE status = 'pending' AND (expires_at >= now() OR kind = 'scheduled_send_held')
 GROUP BY 1 ORDER BY 2 DESC, 1;

\echo '6. Pending contact proposals for addresses on reserved test domains'
SELECT count(*) AS reserved_domain_proposals
  FROM approval
 WHERE status = 'pending' AND kind = 'capture_counterparty' AND expires_at >= now()
   AND (lower(proposed_change ->> 'domain') ~ '(^|\.)(test|example|invalid|localhost)$'
     OR EXISTS (SELECT 1 FROM unnest(ARRAY['example.com', 'example.net', 'example.org']) reserved(host)
                 WHERE lower(proposed_change ->> 'domain') = reserved.host
                    OR lower(proposed_change ->> 'domain') LIKE '%.' || reserved.host));

\echo '7. Seat addresses learned from delivery whose local part names a machine sender'
-- A marker word at either end of the local part, as hasMachineMarker in
-- backend/internal/modules/capture/transactional.go reads it. A lower bound:
-- IsMachineAddress there also knows transactional sending domains.
SELECT count(*) AS machine_aliases
  FROM capture_owner_identity
 WHERE source = 'delivered_to' AND kind = 'address'
   AND split_part(lower(value), '@', 1) ~ ('^(' || :'machine_words' || ')([+._-]|$)|(^|[+._-])('
                                           || :'machine_words' || ')$');

\echo '8. Evidence cited by open deal suggestions on both companies of an open duplicate pair'
SELECT count(DISTINCT (ev.kind, coalesce(ev.activity_id, ev.signal_id, ev.attachment_id))) AS doubled_evidence
  FROM dedupe_candidate pair
  JOIN deal_suggestion ls ON ls.company_id = pair.left_company_id AND ls.state = 'open'
  JOIN deal_suggestion rs ON rs.company_id = pair.right_company_id AND rs.state = 'open'
  JOIN deal_suggestion_evidence ev ON ev.suggestion_id = ls.id
  JOIN deal_suggestion_evidence rev ON rev.suggestion_id = rs.id AND rev.kind = ev.kind
   AND coalesce(rev.activity_id, rev.signal_id, rev.attachment_id)
     = coalesce(ev.activity_id, ev.signal_id, ev.attachment_id)
 WHERE pair.entity_type = 'company' AND pair.disposition = 'open' AND pair.archived_at IS NULL;

ROLLBACK;

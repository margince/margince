SET LOCAL lock_timeout = '3s';
-- Capture used to file every reply under its References root, so a reply that changed
-- the subject stayed in the old conversation. Capture now gives such a reply a
-- conversation of its own (capture/threadboundary.go). This regroups the mail captured
-- before that, by the same rule:
--
--   * A message's parent is the latest earlier message of the same conversation that it
--     references. Mail stores no reference order, so the latest one stands in for
--     In-Reply-To.
--   * Topics compare as the answer check does: reply prefixes and spaces folded, lower
--     case. A blank topic, such as a bare "Re:", takes its parent's topic.
--   * A message whose topic differs from its parent's opens a conversation keyed by its
--     own Message-ID; its replies on that topic follow it. The sender chose that ID, so a
--     key another message already carries leaves the message with its parent.
--
-- Only splits happen: parent and reply already share a key, so no conversation gains a
-- message it did not hold. Each seat's settled verdict is copied to the new key, a consent
-- basis and an outbound delivery move with the message they came from, and the signal
-- scan of every touched conversation starts again. A second run finds every split root
-- without a parent of its key and changes nothing.
--
-- Only a settled verdict is copied. A pending question names a message of the old
-- conversation, and the next mail of the new one opens its own.
--
-- On a large installation, run it per range of thread keys: add
-- `AND a.thread_key >= '<from>' AND a.thread_key < '<to>'` to the first statement. A
-- reply and its parent share a key, so a range never separates them.

CREATE TEMP TABLE regroup_mail ON COMMIT DROP AS
SELECT a.id, a.thread_key, a.occurred_at,
       lower(btrim(regexp_replace(regexp_replace(coalesce(a.subject, ''),
         '^(\s*(re|aw|antw)\s*:)+\s*', '', 'i'), '\s+', ' ', 'g'))) AS topic,
       coalesce(CASE WHEN a.source_system = 'email' THEN btrim(a.source_id, '<>') END,
                (SELECT min(i.identity_key) FROM activity_identity i
                  WHERE i.activity_id = a.id AND i.identity_kind = 'mail')) AS message_id
  FROM activity a
 WHERE a.kind = 'email' AND a.archived_at IS NULL AND a.thread_key <> '';
CREATE INDEX ON regroup_mail (thread_key);

CREATE TEMP TABLE regroup_parent ON COMMIT DROP AS
SELECT DISTINCT ON (c.id) c.id AS child, p.id AS parent
  FROM regroup_mail c
  JOIN activity_mail_reference r ON r.activity_id = c.id
  JOIN regroup_mail p ON p.thread_key = c.thread_key AND (p.occurred_at, p.id) < (c.occurred_at, c.id)
                     AND (p.message_id = r.referenced_id
                          OR EXISTS (SELECT 1 FROM activity_identity i
                                      WHERE i.activity_id = p.id AND i.identity_kind = 'mail'
                                        AND i.identity_key = r.referenced_id))
 ORDER BY c.id, p.occurred_at DESC, p.id DESC;

CREATE TEMP TABLE regroup ON COMMIT DROP AS
WITH RECURSIVE walk AS (
    SELECT m.id, m.thread_key AS new_key, m.topic
      FROM regroup_mail m
     WHERE NOT EXISTS (SELECT 1 FROM regroup_parent pr WHERE pr.child = m.id)
    UNION ALL
    SELECT c.id,
           CASE WHEN c.topic <> '' AND w.topic <> '' AND c.topic <> w.topic AND c.message_id IS NOT NULL
                 AND NOT EXISTS (SELECT 1 FROM activity o WHERE o.thread_key = c.message_id AND o.id <> c.id)
                THEN c.message_id ELSE w.new_key END,
           CASE WHEN c.topic = '' THEN w.topic ELSE c.topic END
      FROM walk w
      JOIN regroup_parent pr ON pr.parent = w.id
      JOIN regroup_mail c ON c.id = pr.child
)
SELECT w.id, m.thread_key AS old_key, w.new_key
  FROM walk w JOIN regroup_mail m ON m.id = w.id
 WHERE w.new_key <> m.thread_key;

INSERT INTO capture_thread_verdict
       (thread_key, user_id, status, kind, confidence, disposition_reason, seen_addresses,
        first_activity_id, resolved_at)
SELECT DISTINCT ON (g.new_key, v.user_id) g.new_key, v.user_id, v.status, v.kind, v.confidence,
       v.disposition_reason, v.seen_addresses, v.first_activity_id, v.resolved_at
  FROM regroup g
  JOIN capture_thread_verdict v ON v.thread_key = g.old_key AND v.status <> 'pending'
 ORDER BY g.new_key, v.user_id
    ON CONFLICT (thread_key, user_id) DO NOTHING;

UPDATE activity a SET thread_key = g.new_key FROM regroup g WHERE a.id = g.id;
UPDATE communication_basis b SET thread_key = g.new_key
  FROM regroup g WHERE b.source_activity_id = g.id AND b.thread_key = g.old_key;
UPDATE comms_outbound o SET thread_key = g.new_key
  FROM regroup g WHERE o.activity_id = g.id AND o.thread_key = g.old_key;
DELETE FROM signal_thread_scan s
 USING (SELECT old_key AS thread_key FROM regroup UNION SELECT new_key FROM regroup) touched
 WHERE s.thread_key = touched.thread_key;

INSERT INTO audit_log (actor_type, actor_id, action, entity_type, entity_id, before, after)
SELECT 'system', 'migration', 'update', 'activity', g.id,
       jsonb_build_object('thread_key', g.old_key),
       jsonb_build_object('thread_key', g.new_key,
         'datafix', '2026-11-03_a_reply_on_a_new_topic_is_its_own_conversation')
  FROM regroup g;

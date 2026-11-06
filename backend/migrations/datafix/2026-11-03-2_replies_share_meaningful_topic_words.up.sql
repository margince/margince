SET LOCAL lock_timeout = '3s';

DO $$ BEGIN
    IF current_setting('margince.reply_topic_repair_approved', true) IS DISTINCT FROM 'on' THEN
        RAISE EXCEPTION 'Topic repair is on hold. Review a rollback preview before setting margince.reply_topic_repair_approved to on.';
    END IF;
END $$;

-- Reference order is not stored, so the latest earlier referenced message stands
-- in for In-Reply-To. A thread-key range keeps every parent with its replies.

CREATE TEMP TABLE regroup_mail ON COMMIT DROP AS
SELECT a.id, a.thread_key, a.occurred_at,
       a.subject AS topic,
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
           CASE WHEN mail_topics_differ(w.topic, c.topic) AND c.message_id IS NOT NULL
                 AND NOT EXISTS (SELECT 1 FROM activity o WHERE o.thread_key = c.message_id AND o.id <> c.id)
                THEN c.message_id ELSE w.new_key END,
           c.topic
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
         'datafix', '2026-11-03-2_replies_share_meaningful_topic_words')
  FROM regroup g;

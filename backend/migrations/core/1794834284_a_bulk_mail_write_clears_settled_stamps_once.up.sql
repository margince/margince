SET LOCAL lock_timeout = '3s';

-- One clear per statement instead of one per row: a mail sync or import that
-- writes N messages ran N UPDATE probes, which put a 95,000-row insert near the
-- 30s statement ceiling. Each thread keeps its own since, the earliest
-- occurred_at among the rows written on it; a NULL time clears every stamp.
CREATE FUNCTION clear_reply_settled_by_thread(threads text[], sinces timestamptz[]) RETURNS void LANGUAGE plpgsql AS $$
DECLARE
  moving text := current_setting('margince.last_activity_move', true);
BEGIN
  PERFORM set_config('margince.last_activity_move', 'on', true);
  UPDATE activity a
     SET reply_settled_at = NULL
    FROM unnest(threads, sinces) AS c(thread_key, since)
   WHERE a.thread_key = c.thread_key
     AND a.direction = 'inbound'
     AND a.kind IN ('email', 'message')
     AND a.restricted_at IS NULL
     AND a.reply_settled_at >= coalesce(c.since, '-infinity');
  PERFORM set_config('margince.last_activity_move', coalesce(moving, ''), true);
END;
$$;

-- The single-since form stays for the participant and contact triggers, and
-- runs the same UPDATE.
CREATE OR REPLACE FUNCTION clear_reply_settled(threads text[], since timestamptz) RETURNS void LANGUAGE plpgsql AS $$
BEGIN
  PERFORM clear_reply_settled_by_thread(threads, array_fill(since, ARRAY[cardinality(threads)]));
END;
$$;

CREATE FUNCTION written_mail_clears_reply_settled() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
  PERFORM clear_reply_settled_by_thread(array_agg(thread_key), array_agg(since))
     FROM (SELECT thread_key,
                  CASE WHEN bool_or(occurred_at IS NULL) THEN NULL ELSE min(occurred_at) END AS since
             FROM written_mail
            WHERE kind IN ('email', 'message') AND thread_key IS NOT NULL
            GROUP BY thread_key) AS threads
   HAVING count(*) > 0;
  RETURN NULL;
END;
$$;

-- An UPDATE keeps its row trigger: transition tables allow neither a column
-- list nor a second event.
CREATE OR REPLACE FUNCTION activity_clears_reply_settled() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
  IF NEW.kind IN ('email', 'message') OR OLD.kind IN ('email', 'message') THEN
    PERFORM clear_reply_settled(ARRAY[NEW.thread_key, OLD.thread_key], least(NEW.occurred_at, OLD.occurred_at));
  END IF;
  RETURN NULL;
END;
$$;

DROP TRIGGER activity_clears_reply_settled ON activity;
CREATE TRIGGER activity_clears_reply_settled
  AFTER UPDATE OF thread_key, kind, channel_provider, direction, archived_at,
    occurred_at, occurred_precision, counterparty_email
  ON activity FOR EACH ROW EXECUTE FUNCTION activity_clears_reply_settled();
CREATE TRIGGER inserted_mail_clears_reply_settled
  AFTER INSERT ON activity REFERENCING NEW TABLE AS written_mail
  FOR EACH STATEMENT EXECUTE FUNCTION written_mail_clears_reply_settled();
CREATE TRIGGER deleted_mail_clears_reply_settled
  AFTER DELETE ON activity REFERENCING OLD TABLE AS written_mail
  FOR EACH STATEMENT EXECUTE FUNCTION written_mail_clears_reply_settled();

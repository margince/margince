SET LOCAL lock_timeout = '3s';

-- When a same-thread fact settled this inbound message: a newer inbound
-- message, or our reply that reached its sender. NULL is not checked yet,
-- 'infinity' is checked and not settled. The owed check reads it before its
-- full clause, so a missing stamp costs time and never an answer. Only the
-- stamping job sets it; the triggers below only clear it. The check ignores
-- the stamp of a restricted row, which no write may touch.
ALTER TABLE activity ADD COLUMN reply_settled_at timestamptz;

-- Clears the stamps of the inbound messages on the threads a change can
-- settle or unsettle, so the job reads them again. A fact at time t can only
-- move a stamp at or after t, so with since given, earlier stamps stay. The
-- stamp is bookkeeping: it moves no version, so a reader's edit in flight is
-- not refused for it. A restricted row is immutable and is never stamped.
CREATE FUNCTION clear_reply_settled(threads text[], since timestamptz) RETURNS void LANGUAGE plpgsql AS $$
DECLARE
  moving text := current_setting('margince.last_activity_move', true);
BEGIN
  PERFORM set_config('margince.last_activity_move', 'on', true);
  UPDATE activity
     SET reply_settled_at = NULL
   WHERE thread_key = ANY(threads)
     AND direction = 'inbound'
     AND kind IN ('email', 'message')
     AND restricted_at IS NULL
     AND reply_settled_at >= coalesce(since, '-infinity');
  PERFORM set_config('margince.last_activity_move', coalesce(moving, ''), true);
END;
$$;

-- Writes the stamps the job read for messages it holds locked. Like the clear,
-- it is bookkeeping that moves no version.
CREATE FUNCTION keep_reply_settled(messages uuid[], settled timestamptz[]) RETURNS bigint LANGUAGE plpgsql AS $$
DECLARE
  moving text := current_setting('margince.last_activity_move', true);
  kept bigint;
BEGIN
  PERFORM set_config('margince.last_activity_move', 'on', true);
  UPDATE activity a
     SET reply_settled_at = s.at
    FROM unnest(messages, settled) AS s(id, at)
   WHERE a.id = s.id AND a.reply_settled_at IS NULL AND a.restricted_at IS NULL;
  GET DIAGNOSTICS kept = ROW_COUNT;
  PERFORM set_config('margince.last_activity_move', coalesce(moving, ''), true);
  RETURN kept;
END;
$$;

CREATE FUNCTION activity_clears_reply_settled() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
  IF TG_OP = 'INSERT' THEN
    IF NEW.kind IN ('email', 'message') THEN
      PERFORM clear_reply_settled(ARRAY[NEW.thread_key], NEW.occurred_at);
    END IF;
  ELSIF TG_OP = 'DELETE' THEN
    IF OLD.kind IN ('email', 'message') THEN
      PERFORM clear_reply_settled(ARRAY[OLD.thread_key], OLD.occurred_at);
    END IF;
  ELSIF NEW.kind IN ('email', 'message') OR OLD.kind IN ('email', 'message') THEN
    PERFORM clear_reply_settled(ARRAY[NEW.thread_key, OLD.thread_key], least(NEW.occurred_at, OLD.occurred_at));
  END IF;
  RETURN NULL;
END;
$$;

-- Every column the thread arms of the owed check read. The stamp itself is not
-- among them, so the job's own write fires nothing.
CREATE TRIGGER activity_clears_reply_settled
  AFTER INSERT OR DELETE OR UPDATE OF thread_key, kind, channel_provider, direction, archived_at,
    occurred_at, occurred_precision, counterparty_email
  ON activity FOR EACH ROW EXECUTE FUNCTION activity_clears_reply_settled();

-- Who a message is from, and who a reply reached or only blind-copied.
CREATE FUNCTION participant_clears_reply_settled() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
  PERFORM clear_reply_settled(ARRAY(
    SELECT a.thread_key FROM activity a
     WHERE a.id IN (CASE WHEN TG_OP = 'DELETE' THEN NULL ELSE NEW.activity_id END,
                    CASE WHEN TG_OP = 'INSERT' THEN NULL ELSE OLD.activity_id END)), NULL);
  RETURN NULL;
END;
$$;

CREATE TRIGGER participant_clears_reply_settled
  AFTER INSERT OR UPDATE OR DELETE ON activity_participant
  FOR EACH ROW EXECUTE FUNCTION participant_clears_reply_settled();

-- A sender's addresses decide which of our replies reached them.
CREATE FUNCTION contact_email_clears_reply_settled() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
  PERFORM clear_reply_settled(ARRAY(
    SELECT DISTINCT a.thread_key FROM activity_participant p
      JOIN activity a ON a.id = p.activity_id
     WHERE p.role = 'from'
       AND p.contact_id IN (CASE WHEN TG_OP = 'DELETE' THEN NULL ELSE NEW.contact_id END,
                            CASE WHEN TG_OP = 'INSERT' THEN NULL ELSE OLD.contact_id END)), NULL);
  RETURN NULL;
END;
$$;

CREATE TRIGGER contact_email_clears_reply_settled
  AFTER INSERT OR UPDATE OR DELETE ON contact_email
  FOR EACH ROW EXECUTE FUNCTION contact_email_clears_reply_settled();

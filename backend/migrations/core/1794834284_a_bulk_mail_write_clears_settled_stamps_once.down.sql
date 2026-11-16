SET LOCAL lock_timeout = '3s';

DROP TRIGGER deleted_mail_clears_reply_settled ON activity;
DROP TRIGGER inserted_mail_clears_reply_settled ON activity;
DROP TRIGGER activity_clears_reply_settled ON activity;
DROP FUNCTION written_mail_clears_reply_settled();

CREATE OR REPLACE FUNCTION clear_reply_settled(threads text[], since timestamptz) RETURNS void LANGUAGE plpgsql AS $$
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
DROP FUNCTION clear_reply_settled_by_thread(text[], timestamptz[]);

CREATE OR REPLACE FUNCTION activity_clears_reply_settled() RETURNS trigger LANGUAGE plpgsql AS $$
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
CREATE TRIGGER activity_clears_reply_settled
  AFTER INSERT OR DELETE OR UPDATE OF thread_key, kind, channel_provider, direction, archived_at,
    occurred_at, occurred_precision, counterparty_email
  ON activity FOR EACH ROW EXECUTE FUNCTION activity_clears_reply_settled();

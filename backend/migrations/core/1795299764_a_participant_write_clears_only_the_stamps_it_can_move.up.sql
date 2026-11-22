SET LOCAL lock_timeout = '3s';

-- A stamp is never before its own message (occurredday.AnswersSQL), and a
-- participant of the message at t is read only by the stamps of mail at or
-- after t. So each thread clears from its earliest written message, once per
-- statement; a NULL time clears every stamp.
CREATE FUNCTION written_participant_clears_reply_settled() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
  PERFORM clear_reply_settled_by_thread(array_agg(thread_key), array_agg(since))
     FROM (SELECT a.thread_key,
                  CASE WHEN bool_or(a.occurred_at IS NULL) THEN NULL ELSE min(a.occurred_at) END AS since
             FROM activity a
            WHERE a.id IN (SELECT activity_id FROM written_participant)
              AND a.kind IN ('email', 'message') AND a.thread_key IS NOT NULL
            GROUP BY a.thread_key) AS threads
   HAVING count(*) > 0;
  RETURN NULL;
END;
$$;

CREATE OR REPLACE FUNCTION participant_clears_reply_settled() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
  PERFORM clear_reply_settled_by_thread(array_agg(thread_key), array_agg(since))
     FROM (SELECT a.thread_key,
                  CASE WHEN bool_or(a.occurred_at IS NULL) THEN NULL ELSE min(a.occurred_at) END AS since
             FROM activity a
            WHERE a.id IN (NEW.activity_id, OLD.activity_id)
              AND a.kind IN ('email', 'message') AND a.thread_key IS NOT NULL
            GROUP BY a.thread_key) AS threads
   HAVING count(*) > 0;
  RETURN NULL;
END;
$$;

-- The participant columns the thread arms of the owed check read.
DROP TRIGGER participant_clears_reply_settled ON activity_participant;
CREATE TRIGGER participant_clears_reply_settled
  AFTER UPDATE OF activity_id, role, address, contact_id
  ON activity_participant FOR EACH ROW EXECUTE FUNCTION participant_clears_reply_settled();
CREATE TRIGGER inserted_participant_clears_reply_settled
  AFTER INSERT ON activity_participant REFERENCING NEW TABLE AS written_participant
  FOR EACH STATEMENT EXECUTE FUNCTION written_participant_clears_reply_settled();
CREATE TRIGGER deleted_participant_clears_reply_settled
  AFTER DELETE ON activity_participant REFERENCING OLD TABLE AS written_participant
  FOR EACH STATEMENT EXECUTE FUNCTION written_participant_clears_reply_settled();

-- A sender's addresses move only the stamps of the mail they sent, each at or
-- after its own time.
CREATE FUNCTION written_contact_email_clears_reply_settled() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
  PERFORM clear_reply_settled_by_thread(array_agg(thread_key), array_agg(since))
     FROM (SELECT a.thread_key,
                  CASE WHEN bool_or(a.occurred_at IS NULL) THEN NULL ELSE min(a.occurred_at) END AS since
             FROM activity_participant p
             JOIN activity a ON a.id = p.activity_id
            WHERE p.role = 'from'
              AND p.contact_id IN (SELECT contact_id FROM written_contact_email)
              AND a.kind IN ('email', 'message') AND a.thread_key IS NOT NULL
            GROUP BY a.thread_key) AS threads
   HAVING count(*) > 0;
  RETURN NULL;
END;
$$;

CREATE OR REPLACE FUNCTION contact_email_clears_reply_settled() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
  PERFORM clear_reply_settled_by_thread(array_agg(thread_key), array_agg(since))
     FROM (SELECT a.thread_key,
                  CASE WHEN bool_or(a.occurred_at IS NULL) THEN NULL ELSE min(a.occurred_at) END AS since
             FROM activity_participant p
             JOIN activity a ON a.id = p.activity_id
            WHERE p.role = 'from'
              AND p.contact_id IN (NEW.contact_id, OLD.contact_id)
              AND a.kind IN ('email', 'message') AND a.thread_key IS NOT NULL
            GROUP BY a.thread_key) AS threads
   HAVING count(*) > 0;
  RETURN NULL;
END;
$$;

-- The contact_email columns the sender arm of the owed check reads.
DROP TRIGGER contact_email_clears_reply_settled ON contact_email;
CREATE TRIGGER contact_email_clears_reply_settled
  AFTER UPDATE OF contact_id, email, archived_at
  ON contact_email FOR EACH ROW EXECUTE FUNCTION contact_email_clears_reply_settled();
CREATE TRIGGER inserted_contact_email_clears_reply_settled
  AFTER INSERT ON contact_email REFERENCING NEW TABLE AS written_contact_email
  FOR EACH STATEMENT EXECUTE FUNCTION written_contact_email_clears_reply_settled();
CREATE TRIGGER deleted_contact_email_clears_reply_settled
  AFTER DELETE ON contact_email REFERENCING OLD TABLE AS written_contact_email
  FOR EACH STATEMENT EXECUTE FUNCTION written_contact_email_clears_reply_settled();

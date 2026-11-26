SET LOCAL lock_timeout = '3s';

DROP TRIGGER deleted_contact_email_clears_reply_settled ON contact_email;
DROP TRIGGER inserted_contact_email_clears_reply_settled ON contact_email;
DROP TRIGGER contact_email_clears_reply_settled ON contact_email;
DROP FUNCTION written_contact_email_clears_reply_settled();

DROP TRIGGER deleted_participant_clears_reply_settled ON activity_participant;
DROP TRIGGER inserted_participant_clears_reply_settled ON activity_participant;
DROP TRIGGER participant_clears_reply_settled ON activity_participant;
DROP FUNCTION written_participant_clears_reply_settled();

CREATE OR REPLACE FUNCTION participant_clears_reply_settled() RETURNS trigger LANGUAGE plpgsql AS $$
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

CREATE OR REPLACE FUNCTION contact_email_clears_reply_settled() RETURNS trigger LANGUAGE plpgsql AS $$
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

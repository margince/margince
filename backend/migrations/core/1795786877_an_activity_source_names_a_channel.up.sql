-- An activity row says which channel it arrived by, and source_system names the
-- writer an older source spelled. The check is unvalidated; a data task moves older rows.
SET LOCAL lock_timeout = '3s';

ALTER TABLE activity ADD CONSTRAINT activity_source_check
    CHECK (source IN ('api', 'automation', 'capture', 'import', 'manual', 'webform')) NOT VALID;

-- Names the product's own writer an older source spelled, or NULL when it named none.
-- The system's own word names none: a task with no source_system is read as current work.
CREATE FUNCTION activity_source_producer(source text) RETURNS text
LANGUAGE sql IMMUTABLE AS $$
 SELECT CASE
  WHEN source IN ('attachment_access_request', 'attachment_extraction_accept', 'chat_links', 'chat_screenshot',
                  'commitment', 'deal_room', 'email_request', 'meeting_notes', 'meeting_notes_mail',
                  'overnight-reconcile', 'privacy_request', 'public_booking', 'web_form', 'which_deal')
   THEN source
 END
$$;

-- Maps an older activity source onto its channel. A connector wrote a captured
-- row, and its source spelled the connector and the message.
CREATE FUNCTION activity_source_channel(source text, captured_by text) RETURNS text
LANGUAGE sql IMMUTABLE AS $$
 SELECT CASE
  WHEN record_source_channel(source) IN (source, 'import') THEN record_source_channel(source)
  WHEN source IN ('chat_screenshot', 'meeting_notes', 'meeting_notes_mail') OR captured_by LIKE 'connector:%' THEN 'capture'
  WHEN source IN ('public_booking', 'web_form') THEN 'webform'
  WHEN source IN ('', 'system') OR activity_source_producer(source) IS NOT NULL THEN 'automation'
  ELSE record_source_channel(source)
 END
$$;

-- The source_system an older row keeps once its source names a channel. It takes
-- the writer's name only where it is free: a source_id makes it a replay key, and
-- a task with none is read as current work.
CREATE FUNCTION activity_source_system(source text, source_system text, source_id text, kind text) RETURNS text
LANGUAGE sql IMMUTABLE AS $$
 SELECT CASE
  WHEN source_system IS NULL AND source_id IS NULL AND kind <> 'task' THEN activity_source_producer(source)
  ELSE source_system
 END
$$;

-- Internal writers still pass the older words, and an older row is judged on its
-- next update. Both land on the channel.
CREATE FUNCTION move_activity_source_onto_its_channel() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
 NEW.source_system := activity_source_system(NEW.source, NEW.source_system, NEW.source_id, NEW.kind);
 NEW.source := activity_source_channel(NEW.source, NEW.captured_by);
 RETURN NEW;
END;
$$;

CREATE TRIGGER activity_source_channel
 BEFORE INSERT OR UPDATE ON activity
 FOR EACH ROW WHEN (NEW.source NOT IN ('api', 'automation', 'capture', 'import', 'manual', 'webform'))
 EXECUTE FUNCTION move_activity_source_onto_its_channel();

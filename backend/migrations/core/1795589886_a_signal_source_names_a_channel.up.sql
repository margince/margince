-- A signal row says which channel it arrived by, and source_system names the
-- producer that wrote it. The check is unvalidated; a data task moves older rows.
SET LOCAL lock_timeout = '3s';

ALTER TABLE signal ADD COLUMN source_system text;
COMMENT ON COLUMN signal.source_system IS
    'The producer that wrote the signal: signal-scan, site-newsroom, technical-lookup or the name of an extension. NULL when a member or an API client filed it.';

ALTER TABLE signal ADD CONSTRAINT signal_source_check
    CHECK (source IN ('api', 'automation', 'capture', 'import', 'manual', 'webform')) NOT VALID;

-- Names the producer an older source held, or NULL when a client wrote the text.
-- An extension signal is known by its kind, and held its extension's name.
-- That name can spell a channel, so the kind decides and not the word.
CREATE FUNCTION signal_source_producer(source text, kind text) RETURNS text
LANGUAGE sql IMMUTABLE AS $$
 SELECT CASE
  WHEN source IN ('signal-scan', 'site-newsroom', 'technical-lookup') THEN source
  WHEN kind LIKE 'ext:%' THEN source
 END
$$;

-- Maps a signal source written before the list onto its channel.
CREATE FUNCTION signal_source_channel(source text, kind text) RETURNS text
LANGUAGE sql IMMUTABLE AS $$
 SELECT CASE
  WHEN signal_source_producer(source, kind) IS NOT NULL THEN 'automation'
  ELSE record_source_channel(source)
 END
$$;

-- An unvalidated check still judges an older row on its next update. This moves
-- the producer into source_system first, so a triage before the data task succeeds.
CREATE FUNCTION move_signal_source_onto_its_channel() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
 NEW.source_system := COALESCE(NEW.source_system, signal_source_producer(NEW.source, NEW.kind));
 NEW.source := signal_source_channel(NEW.source, NEW.kind);
 RETURN NEW;
END;
$$;

CREATE TRIGGER signal_source_channel
 BEFORE UPDATE ON signal
 FOR EACH ROW WHEN (NEW.source NOT IN ('api', 'automation', 'capture', 'import', 'manual', 'webform')
                    OR (NEW.kind LIKE 'ext:%' AND NEW.source_system IS NULL))
 EXECUTE FUNCTION move_signal_source_onto_its_channel();

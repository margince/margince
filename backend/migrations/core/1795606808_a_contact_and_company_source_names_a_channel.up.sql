-- A contact or company row says which channel it arrived by, and source_system
-- names the connector or writer. The checks are unvalidated; a data task moves older rows.
SET LOCAL lock_timeout = '3s';

ALTER TABLE contact ADD CONSTRAINT contact_source_check
    CHECK (source IN ('api', 'automation', 'capture', 'import', 'manual', 'webform')) NOT VALID;
ALTER TABLE company ADD CONSTRAINT company_source_check
    CHECK (source IN ('api', 'automation', 'capture', 'import', 'manual', 'webform')) NOT VALID;

-- Names the connector or writer an older source spelled, or NULL when it named none.
-- An import's stamp is <system>:<object>:<id>, and the system is kept.
CREATE FUNCTION party_source_producer(source text) RETURNS text
LANGUAGE sql IMMUTABLE AS $$
 SELECT CASE
  WHEN source LIKE 'domain_triage:%' THEN 'domain_triage'
  WHEN source LIKE 'mirror:%' THEN substring(source FROM '^(mirror:.*?):(?:company|contact|lead):')
  WHEN source LIKE 'ext:%' THEN source
  WHEN split_part(source, ':', 1) IN ('dispact', 'gcal', 'gdocs', 'gmail', 'graph', 'graphcal', 'imap',
                                      'offline_demo', 'telegram', 'test_mailbox') THEN split_part(source, ':', 1)
  WHEN source IN ('capture_counterparty', 'capture_counterparty_verdict', 'coldstart', 'erasure_return',
                  'public_booking', 'siteread', 'vcard_import', 'web_form')
   THEN source
 END
$$;

-- Maps an older contact or company source onto its channel. An extension's
-- word is an import when the writer also named the system, else a capture.
CREATE FUNCTION party_source_channel(source text, source_system text) RETURNS text
LANGUAGE sql IMMUTABLE AS $$
 SELECT CASE
  WHEN source LIKE 'mirror:%' OR (source LIKE 'ext:%' AND source_system IS NOT NULL) THEN 'import'
  WHEN party_source_producer(source) IN ('erasure_return', 'public_booking', 'web_form') THEN 'webform'
  WHEN party_source_producer(source) = 'vcard_import' THEN 'import'
  WHEN party_source_producer(source) = 'coldstart' THEN 'automation'
  WHEN party_source_producer(source) IS NOT NULL THEN 'capture'
  ELSE record_source_channel(source)
 END
$$;

-- Internal writers still pass the older words, and an older row is judged on its
-- next update. Both land on the channel, with the writer's name in source_system.
CREATE FUNCTION move_party_source_onto_its_channel() RETURNS trigger
LANGUAGE plpgsql AS $$
DECLARE
 channel text := party_source_channel(NEW.source, NEW.source_system);
BEGIN
 NEW.source_system := COALESCE(NEW.source_system, party_source_producer(NEW.source));
 NEW.source := channel;
 RETURN NEW;
END;
$$;

CREATE TRIGGER contact_source_channel
 BEFORE INSERT OR UPDATE ON contact
 FOR EACH ROW WHEN (NEW.source NOT IN ('api', 'automation', 'capture', 'import', 'manual', 'webform'))
 EXECUTE FUNCTION move_party_source_onto_its_channel();
CREATE TRIGGER company_source_channel
 BEFORE INSERT OR UPDATE ON company
 FOR EACH ROW WHEN (NEW.source NOT IN ('api', 'automation', 'capture', 'import', 'manual', 'webform'))
 EXECUTE FUNCTION move_party_source_onto_its_channel();

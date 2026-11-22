-- A deal, offer, project or partner row says which channel it arrived by. The
-- checks are added unvalidated; a data task moves older rows onto the list.
SET LOCAL lock_timeout = '3s';

-- A partner row is written from the partner form, so an older value is manual.
UPDATE partner SET source = 'manual'
 WHERE source NOT IN ('api', 'automation', 'capture', 'import', 'manual', 'webform');

ALTER TABLE deal ADD CONSTRAINT deal_source_check
    CHECK (source IN ('api', 'automation', 'capture', 'import', 'manual', 'webform')) NOT VALID;
ALTER TABLE offer ADD CONSTRAINT offer_source_check
    CHECK (source IN ('api', 'automation', 'capture', 'import', 'manual', 'webform')) NOT VALID;
ALTER TABLE project ADD CONSTRAINT project_source_check
    CHECK (source IN ('api', 'automation', 'capture', 'import', 'manual', 'webform')) NOT VALID;
ALTER TABLE partner ADD CONSTRAINT partner_source_check
    CHECK (source IN ('api', 'automation', 'capture', 'import', 'manual', 'webform')) NOT VALID;

-- Maps a source written before the list onto its channel. A value it does not
-- know came through the public create request, which took free text.
CREATE FUNCTION record_source_channel(source text) RETURNS text
LANGUAGE sql IMMUTABLE AS $$
 SELECT CASE
  WHEN source IN ('api', 'automation', 'capture', 'import', 'manual', 'webform') THEN source
  WHEN source IN ('mcp', 'ui') OR source LIKE 'lead:%' THEN 'manual'
  WHEN source LIKE 'mirror:%' OR source LIKE 'ext:%' OR source LIKE 'import:%' THEN 'import'
  WHEN source LIKE 'agent:%' THEN 'automation'
  ELSE 'api'
 END
$$;

-- An unvalidated check still judges an older row on its next update. This moves
-- such a row onto its channel first, so an edit before the data task succeeds.
CREATE FUNCTION move_source_onto_its_channel() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
 NEW.source := record_source_channel(NEW.source);
 RETURN NEW;
END;
$$;

CREATE TRIGGER deal_source_channel
 BEFORE UPDATE ON deal
 FOR EACH ROW WHEN (NEW.source NOT IN ('api', 'automation', 'capture', 'import', 'manual', 'webform'))
 EXECUTE FUNCTION move_source_onto_its_channel();
CREATE TRIGGER offer_source_channel
 BEFORE UPDATE ON offer
 FOR EACH ROW WHEN (NEW.source NOT IN ('api', 'automation', 'capture', 'import', 'manual', 'webform'))
 EXECUTE FUNCTION move_source_onto_its_channel();
CREATE TRIGGER project_source_channel
 BEFORE UPDATE ON project
 FOR EACH ROW WHEN (NEW.source NOT IN ('api', 'automation', 'capture', 'import', 'manual', 'webform'))
 EXECUTE FUNCTION move_source_onto_its_channel();

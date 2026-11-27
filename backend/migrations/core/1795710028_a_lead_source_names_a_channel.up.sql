-- A lead says which channel it arrived by, through the mapping contacts and
-- companies use, and the administered list of lead sources is retired.
SET LOCAL lock_timeout = '3s';

ALTER TABLE lead ADD CONSTRAINT lead_source_check
    CHECK (source IN ('api', 'automation', 'capture', 'import', 'manual', 'webform')) NOT VALID;

-- A lead is promoted into a contact, so both read an older word the same way.
-- Its source_system is the replay key guards read, so the word never fills it.
CREATE FUNCTION move_lead_source_onto_its_channel() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
 NEW.source := party_source_channel(NEW.source, NEW.source_system);
 RETURN NEW;
END;
$$;

CREATE TRIGGER lead_source_channel
 BEFORE INSERT OR UPDATE ON lead
 FOR EACH ROW WHEN (NEW.source NOT IN ('api', 'automation', 'capture', 'import', 'manual', 'webform'))
 EXECUTE FUNCTION move_lead_source_onto_its_channel();

DROP TABLE lead_source;

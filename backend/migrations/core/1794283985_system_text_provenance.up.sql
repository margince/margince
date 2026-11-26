SET LOCAL lock_timeout = '3s';
ALTER TABLE activity ADD COLUMN system_text jsonb;
ALTER TABLE notice ADD COLUMN system_text jsonb;
ALTER TABLE activity ADD CONSTRAINT activity_system_text_shape CHECK (system_text IS NULL OR (jsonb_typeof(system_text) = 'object' AND (system_text->>'schema_version' = '1') IS TRUE AND octet_length(system_text::text) <= 8192)) NOT VALID;
ALTER TABLE notice ADD CONSTRAINT notice_system_text_shape CHECK (system_text IS NULL OR (jsonb_typeof(system_text) = 'object' AND (system_text->>'schema_version' = '1') IS TRUE AND octet_length(system_text::text) <= 8192)) NOT VALID;

CREATE FUNCTION retire_changed_system_text() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF NEW.subject IS DISTINCT FROM OLD.subject AND
    NEW.system_text->'subject'->>'digest' IS DISTINCT FROM encode(sha256(convert_to(coalesce(NEW.subject, ''), 'UTF8')), 'hex') THEN
  NEW.system_text := NEW.system_text - 'subject';
 END IF;
 IF NEW.body IS DISTINCT FROM OLD.body AND
    NEW.system_text->'body'->>'digest' IS DISTINCT FROM encode(sha256(convert_to(coalesce(NEW.body, ''), 'UTF8')), 'hex') THEN
  NEW.system_text := NEW.system_text - 'body';
 END IF;
 IF NOT (NEW.system_text ? 'subject' OR NEW.system_text ? 'body') THEN NEW.system_text := NULL; END IF;
 RETURN NEW;
END;
$$;
CREATE TRIGGER activity_system_text_retire BEFORE UPDATE ON activity FOR EACH ROW EXECUTE FUNCTION retire_changed_system_text();
CREATE TRIGGER notice_system_text_retire BEFORE UPDATE ON notice FOR EACH ROW EXECUTE FUNCTION retire_changed_system_text();

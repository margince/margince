-- A signal goes when the deal, company, contact or project it is about goes.
--
-- entity_id names a row in one of four tables, and no foreign key can span
-- them. Each branch now has a key column of its own with ON DELETE CASCADE.
-- The pair stays the column readers use.
--
-- The table collects every detection, so the keys must not rewrite it. These
-- are plain columns, which ALTER TABLE adds from the catalog alone, and a
-- trigger fills them from the pair. Every row already here reads
-- entity_unkeyed = true through the column's added-default value, so its keys
-- stay NULL until the data task keys it. An update may clear the marker and
-- never set it, which is how that task keys a row whose record still exists.
--
-- The CHECKs bind a keyed row to its pair, so a writer that bypassed the
-- trigger is refused rather than mis-keyed. They are validated in a later file.
SET LOCAL lock_timeout = '3s';

ALTER TABLE signal
	ADD COLUMN entity_unkeyed    boolean NOT NULL DEFAULT true,
	ADD COLUMN entity_deal_id    uuid,
	ADD COLUMN entity_company_id uuid,
	ADD COLUMN entity_contact_id uuid,
	ADD COLUMN entity_project_id uuid;
ALTER TABLE signal ALTER COLUMN entity_unkeyed SET DEFAULT false;

ALTER TABLE signal
	ADD CONSTRAINT signal_entity_deal_id_fkey FOREIGN KEY (entity_deal_id)
		REFERENCES deal(id) ON DELETE CASCADE NOT VALID,
	ADD CONSTRAINT signal_entity_company_id_fkey FOREIGN KEY (entity_company_id)
		REFERENCES company(id) ON DELETE CASCADE NOT VALID,
	ADD CONSTRAINT signal_entity_contact_id_fkey FOREIGN KEY (entity_contact_id)
		REFERENCES contact(id) ON DELETE CASCADE NOT VALID,
	ADD CONSTRAINT signal_entity_project_id_fkey FOREIGN KEY (entity_project_id)
		REFERENCES project(id) ON DELETE CASCADE NOT VALID,
	ADD CONSTRAINT signal_entity_deal_id_bound
		CHECK (entity_unkeyed OR entity_deal_id IS NOT DISTINCT FROM
			CASE WHEN entity_type = 'deal' THEN entity_id END) NOT VALID,
	ADD CONSTRAINT signal_entity_company_id_bound
		CHECK (entity_unkeyed OR entity_company_id IS NOT DISTINCT FROM
			CASE WHEN entity_type = 'company' THEN entity_id END) NOT VALID,
	ADD CONSTRAINT signal_entity_contact_id_bound
		CHECK (entity_unkeyed OR entity_contact_id IS NOT DISTINCT FROM
			CASE WHEN entity_type = 'contact' THEN entity_id END) NOT VALID,
	ADD CONSTRAINT signal_entity_project_id_bound
		CHECK (entity_unkeyed OR entity_project_id IS NOT DISTINCT FROM
			CASE WHEN entity_type = 'project' THEN entity_id END) NOT VALID,
	-- What refuses an entity type with no key column of its own: a fifth type
	-- added to the type CHECK without a fifth column cannot be written.
	ADD CONSTRAINT signal_entity_shape
		CHECK (entity_unkeyed OR entity_id IS NULL OR num_nonnulls(
			entity_deal_id, entity_company_id, entity_contact_id, entity_project_id) = 1) NOT VALID;

-- The trigger, not the writer, decides whether a row is keyed: an insert is
-- always keyed, and an update keeps the marker or clears it.
CREATE FUNCTION trg_signal_entity_keys() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
	IF TG_OP = 'INSERT' THEN
		NEW.entity_unkeyed := false;
	ELSE
		NEW.entity_unkeyed := OLD.entity_unkeyed AND NEW.entity_unkeyed;
	END IF;
	IF NOT NEW.entity_unkeyed THEN
		NEW.entity_deal_id    := CASE WHEN NEW.entity_type = 'deal'    THEN NEW.entity_id END;
		NEW.entity_company_id := CASE WHEN NEW.entity_type = 'company' THEN NEW.entity_id END;
		NEW.entity_contact_id := CASE WHEN NEW.entity_type = 'contact' THEN NEW.entity_id END;
		NEW.entity_project_id := CASE WHEN NEW.entity_type = 'project' THEN NEW.entity_id END;
	END IF;
	RETURN NEW;
END $$;

CREATE TRIGGER trg_signal_entity_keys
	BEFORE INSERT OR UPDATE ON signal
	FOR EACH ROW EXECUTE FUNCTION trg_signal_entity_keys();


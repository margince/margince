-- A user's last-seen mark on a company or contact goes when the record goes.
--
-- entity_id names a row in one of two tables, and no foreign key can span
-- them. Each branch now has a key column of its own with ON DELETE CASCADE.
-- The pair stays the column readers use, and company360.RecordVisit stays its
-- one writer: the trigger fills the keys from the pair it writes.
--
-- The keys are plain columns a trigger fills, as on signal, so the table is
-- not rewritten. Rows already here read entity_unkeyed = true until the data
-- task keys them. An update may clear the marker and never set it.
SET LOCAL lock_timeout = '3s';

ALTER TABLE user_record_view
	ADD COLUMN entity_unkeyed    boolean NOT NULL DEFAULT true,
	ADD COLUMN entity_company_id uuid,
	ADD COLUMN entity_contact_id uuid;
ALTER TABLE user_record_view ALTER COLUMN entity_unkeyed SET DEFAULT false;

ALTER TABLE user_record_view
	ADD CONSTRAINT user_record_view_entity_company_id_fkey FOREIGN KEY (entity_company_id)
		REFERENCES company(id) ON DELETE CASCADE NOT VALID,
	ADD CONSTRAINT user_record_view_entity_contact_id_fkey FOREIGN KEY (entity_contact_id)
		REFERENCES contact(id) ON DELETE CASCADE NOT VALID,
	ADD CONSTRAINT user_record_view_entity_company_id_bound
		CHECK (entity_unkeyed OR entity_company_id IS NOT DISTINCT FROM
			CASE WHEN entity_type = 'company' THEN entity_id END) NOT VALID,
	ADD CONSTRAINT user_record_view_entity_contact_id_bound
		CHECK (entity_unkeyed OR entity_contact_id IS NOT DISTINCT FROM
			CASE WHEN entity_type = 'contact' THEN entity_id END) NOT VALID,
	ADD CONSTRAINT user_record_view_entity_shape
		CHECK (entity_unkeyed OR num_nonnulls(entity_company_id, entity_contact_id) = 1) NOT VALID;

CREATE FUNCTION trg_user_record_view_entity_keys() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
	IF TG_OP = 'INSERT' THEN
		NEW.entity_unkeyed := false;
	ELSE
		NEW.entity_unkeyed := OLD.entity_unkeyed AND NEW.entity_unkeyed;
	END IF;
	IF NOT NEW.entity_unkeyed THEN
		NEW.entity_company_id := CASE WHEN NEW.entity_type = 'company' THEN NEW.entity_id END;
		NEW.entity_contact_id := CASE WHEN NEW.entity_type = 'contact' THEN NEW.entity_id END;
	END IF;
	RETURN NEW;
END $$;

CREATE TRIGGER trg_user_record_view_entity_keys
	BEFORE INSERT OR UPDATE ON user_record_view
	FOR EACH ROW EXECUTE FUNCTION trg_user_record_view_entity_keys();

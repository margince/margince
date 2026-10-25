-- A weekly commitment lets go of the deal, lead, contact, company or project
-- it links to when that record is deleted, and the commitment stays.
--
-- linked_record_id names a row in one of five tables, and no foreign key can
-- span them. Each branch now has a key column of its own. Its key is ON DELETE
-- SET NULL, not CASCADE: a commitment is the rep's record of the work they
-- undertook that week, and the privacy eraser redacts it rather than deleting
-- it for the same reason. The trigger reads a key the database cleared while
-- the pair stood still as the record going, and clears the pair with it. It
-- must: the SET NULL's own update is not checked against the key again, so a
-- trigger that filled the key from the standing pair would leave it dangling.
--
-- The keys are plain columns a trigger fills, as on signal, so the table is
-- not rewritten. Rows already here read linked_record_unkeyed = true until the
-- data task keys them. An update may clear the marker and never set it.
SET LOCAL lock_timeout = '3s';

ALTER TABLE weekly_plan_commitment
	ADD COLUMN linked_record_unkeyed boolean NOT NULL DEFAULT true,
	ADD COLUMN linked_deal_id        uuid,
	ADD COLUMN linked_lead_id        uuid,
	ADD COLUMN linked_contact_id     uuid,
	ADD COLUMN linked_company_id     uuid,
	ADD COLUMN linked_project_id     uuid;
ALTER TABLE weekly_plan_commitment ALTER COLUMN linked_record_unkeyed SET DEFAULT false;

ALTER TABLE weekly_plan_commitment
	ADD CONSTRAINT weekly_plan_commitment_linked_deal_id_fkey FOREIGN KEY (linked_deal_id)
		REFERENCES deal(id) ON DELETE SET NULL NOT VALID,
	ADD CONSTRAINT weekly_plan_commitment_linked_lead_id_fkey FOREIGN KEY (linked_lead_id)
		REFERENCES lead(id) ON DELETE SET NULL NOT VALID,
	ADD CONSTRAINT weekly_plan_commitment_linked_contact_id_fkey FOREIGN KEY (linked_contact_id)
		REFERENCES contact(id) ON DELETE SET NULL NOT VALID,
	ADD CONSTRAINT weekly_plan_commitment_linked_company_id_fkey FOREIGN KEY (linked_company_id)
		REFERENCES company(id) ON DELETE SET NULL NOT VALID,
	ADD CONSTRAINT weekly_plan_commitment_linked_project_id_fkey FOREIGN KEY (linked_project_id)
		REFERENCES project(id) ON DELETE SET NULL NOT VALID,
	ADD CONSTRAINT weekly_plan_commitment_linked_deal_id_bound
		CHECK (linked_record_unkeyed OR linked_deal_id IS NOT DISTINCT FROM
			CASE WHEN linked_record_type = 'deal' THEN linked_record_id END) NOT VALID,
	ADD CONSTRAINT weekly_plan_commitment_linked_lead_id_bound
		CHECK (linked_record_unkeyed OR linked_lead_id IS NOT DISTINCT FROM
			CASE WHEN linked_record_type = 'lead' THEN linked_record_id END) NOT VALID,
	ADD CONSTRAINT weekly_plan_commitment_linked_contact_id_bound
		CHECK (linked_record_unkeyed OR linked_contact_id IS NOT DISTINCT FROM
			CASE WHEN linked_record_type = 'contact' THEN linked_record_id END) NOT VALID,
	ADD CONSTRAINT weekly_plan_commitment_linked_company_id_bound
		CHECK (linked_record_unkeyed OR linked_company_id IS NOT DISTINCT FROM
			CASE WHEN linked_record_type = 'company' THEN linked_record_id END) NOT VALID,
	ADD CONSTRAINT weekly_plan_commitment_linked_project_id_bound
		CHECK (linked_record_unkeyed OR linked_project_id IS NOT DISTINCT FROM
			CASE WHEN linked_record_type = 'project' THEN linked_record_id END) NOT VALID,
	ADD CONSTRAINT weekly_plan_commitment_linked_record_shape
		CHECK (linked_record_unkeyed OR linked_record_id IS NULL OR num_nonnulls(
			linked_deal_id, linked_lead_id, linked_contact_id, linked_company_id, linked_project_id) = 1) NOT VALID;

CREATE INDEX idx_weekly_plan_commitment_linked_deal    ON weekly_plan_commitment (linked_deal_id)    WHERE linked_deal_id IS NOT NULL;
CREATE INDEX idx_weekly_plan_commitment_linked_lead    ON weekly_plan_commitment (linked_lead_id)    WHERE linked_lead_id IS NOT NULL;
CREATE INDEX idx_weekly_plan_commitment_linked_contact ON weekly_plan_commitment (linked_contact_id) WHERE linked_contact_id IS NOT NULL;
CREATE INDEX idx_weekly_plan_commitment_linked_company ON weekly_plan_commitment (linked_company_id) WHERE linked_company_id IS NOT NULL;
CREATE INDEX idx_weekly_plan_commitment_linked_project ON weekly_plan_commitment (linked_project_id) WHERE linked_project_id IS NOT NULL;

CREATE FUNCTION trg_weekly_plan_commitment_linked_keys() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
	IF TG_OP = 'INSERT' THEN
		NEW.linked_record_unkeyed := false;
	ELSE
		NEW.linked_record_unkeyed := OLD.linked_record_unkeyed AND NEW.linked_record_unkeyed;
	END IF;
	IF NOT NEW.linked_record_unkeyed THEN
		-- Only the SET NULL of a deleted record clears a key and leaves the pair.
		IF TG_OP = 'UPDATE' AND NEW.linked_record_id IS NOT DISTINCT FROM OLD.linked_record_id
		   AND num_nonnulls(OLD.linked_deal_id, OLD.linked_lead_id, OLD.linked_contact_id,
		                    OLD.linked_company_id, OLD.linked_project_id) = 1
		   AND num_nonnulls(NEW.linked_deal_id, NEW.linked_lead_id, NEW.linked_contact_id,
		                    NEW.linked_company_id, NEW.linked_project_id) = 0 THEN
			NEW.linked_record_type := NULL;
			NEW.linked_record_id := NULL;
		END IF;
		NEW.linked_deal_id    := CASE WHEN NEW.linked_record_type = 'deal'    THEN NEW.linked_record_id END;
		NEW.linked_lead_id    := CASE WHEN NEW.linked_record_type = 'lead'    THEN NEW.linked_record_id END;
		NEW.linked_contact_id := CASE WHEN NEW.linked_record_type = 'contact' THEN NEW.linked_record_id END;
		NEW.linked_company_id := CASE WHEN NEW.linked_record_type = 'company' THEN NEW.linked_record_id END;
		NEW.linked_project_id := CASE WHEN NEW.linked_record_type = 'project' THEN NEW.linked_record_id END;
	END IF;
	RETURN NEW;
END $$;

CREATE TRIGGER trg_weekly_plan_commitment_linked_keys
	BEFORE INSERT OR UPDATE ON weekly_plan_commitment
	FOR EACH ROW EXECUTE FUNCTION trg_weekly_plan_commitment_linked_keys();

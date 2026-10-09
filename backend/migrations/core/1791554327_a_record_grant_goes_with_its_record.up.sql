-- A grant on a record goes when the record goes.
--
-- record_id names a row in one of five tables, and no foreign key can span
-- them, so a deleted record left its grants pointing at nothing. Each branch
-- now has a key column of its own, which the database derives from the pair:
-- every writer fills it without saying so, and every row already in the table
-- has it the moment this runs, so there is nothing to backfill. The pair stays
-- the column readers use.
--
-- The keys are NOT VALID because an installation may already hold a grant
-- whose record was deleted before this; a validating scan would fail the
-- migration there. They bind every new row, and a delete cascades for every
-- grant whose record is still present.
--
-- Adding a stored generated column rewrites the table under ACCESS EXCLUSIVE.
-- A grant is a hand-made row, so the table is small.
SET LOCAL lock_timeout = '3s';

ALTER TABLE record_grant
	ADD COLUMN contact_id uuid GENERATED ALWAYS AS (CASE WHEN record_type = 'contact' THEN record_id END) STORED,
	ADD COLUMN company_id uuid GENERATED ALWAYS AS (CASE WHEN record_type = 'company' THEN record_id END) STORED,
	ADD COLUMN deal_id    uuid GENERATED ALWAYS AS (CASE WHEN record_type = 'deal'    THEN record_id END) STORED,
	ADD COLUMN lead_id    uuid GENERATED ALWAYS AS (CASE WHEN record_type = 'lead'    THEN record_id END) STORED,
	ADD COLUMN project_id uuid GENERATED ALWAYS AS (CASE WHEN record_type = 'project' THEN record_id END) STORED;

ALTER TABLE record_grant
	ADD CONSTRAINT record_grant_contact_id_fkey FOREIGN KEY (contact_id) REFERENCES contact(id) ON DELETE CASCADE NOT VALID,
	ADD CONSTRAINT record_grant_company_id_fkey FOREIGN KEY (company_id) REFERENCES company(id) ON DELETE CASCADE NOT VALID,
	ADD CONSTRAINT record_grant_deal_id_fkey    FOREIGN KEY (deal_id)    REFERENCES deal(id)    ON DELETE CASCADE NOT VALID,
	ADD CONSTRAINT record_grant_lead_id_fkey    FOREIGN KEY (lead_id)    REFERENCES lead(id)    ON DELETE CASCADE NOT VALID,
	ADD CONSTRAINT record_grant_project_id_fkey FOREIGN KEY (project_id) REFERENCES project(id) ON DELETE CASCADE NOT VALID;

-- Every row lands in exactly one key column. It holds by construction for
-- every row the type CHECK admits, and it is what refuses a record type with no
-- key column of its own: a sixth type added to that CHECK without a sixth
-- column cannot be written. Validated in a file of its own.
ALTER TABLE record_grant
	ADD CONSTRAINT record_grant_record_shape
	CHECK (num_nonnulls(contact_id, company_id, deal_id, lead_id, project_id) = 1) NOT VALID;

CREATE INDEX idx_record_grant_contact ON record_grant (contact_id) WHERE contact_id IS NOT NULL;
CREATE INDEX idx_record_grant_company ON record_grant (company_id) WHERE company_id IS NOT NULL;
CREATE INDEX idx_record_grant_deal    ON record_grant (deal_id)    WHERE deal_id IS NOT NULL;
CREATE INDEX idx_record_grant_lead    ON record_grant (lead_id)    WHERE lead_id IS NOT NULL;
CREATE INDEX idx_record_grant_project ON record_grant (project_id) WHERE project_id IS NOT NULL;

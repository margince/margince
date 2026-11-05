-- The company a human confirmed this lead belongs to. NULL until somebody
-- confirms one; company_name stays the free text the lead was captured with.
-- SET NULL, because deleting a company must not delete the leads that named it.
--
-- A plain column, which ALTER TABLE adds from the catalog alone. The key is
-- added NOT VALID and validated in a later file, so this lock scans no row.
SET LOCAL lock_timeout = '3s';

ALTER TABLE lead ADD COLUMN company_id uuid;
ALTER TABLE lead
	ADD CONSTRAINT lead_company_id_fkey FOREIGN KEY (company_id)
		REFERENCES company(id) ON DELETE SET NULL NOT VALID;

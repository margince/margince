-- Restores both cascades exactly as the catalog held them, NOT VALID for the
-- reason the up file states, and validated by this file's own pair.
SET LOCAL lock_timeout = '3s';

ALTER TABLE deal DROP CONSTRAINT deal_acquisition_source_fkey;
ALTER TABLE deal
	ADD CONSTRAINT deal_acquisition_source_fkey
		FOREIGN KEY (acquisition_source) REFERENCES deal_acquisition_source (key)
		ON UPDATE CASCADE ON DELETE RESTRICT NOT VALID;

ALTER TABLE lead DROP CONSTRAINT lead_origin_fkey;
ALTER TABLE lead
	ADD CONSTRAINT lead_origin_fkey
		FOREIGN KEY (origin) REFERENCES lead_origin (key)
		ON UPDATE CASCADE ON DELETE RESTRICT NOT VALID;

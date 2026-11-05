-- No foreign key rewrites a child row, and two of them did.
--
-- deal.acquisition_source and lead.origin each pointed at a text-key catalog
-- with ON UPDATE CASCADE. The lead one is the newer of the two and copies the
-- deal one's shape. A rename of the catalog key would have had
-- the DATABASE write every deal carrying it: the row changes with no
-- application code running, so there is no audit_log row and no event_outbox
-- row, which is the write shape AGENTS.md states. trg_deal_updated fires on
-- the cascade as well, so deal.version moves and every client holding the old
-- version gets a 409 it cannot explain.
--
-- RESTRICT instead, which is what the code already assumes: the key is written
-- at insert and never patched, and acquisitionsource.go says a key a deal has
-- ever carried must stay resolvable. A rename now needs a deliberate migration
-- rather than one line added to a patch.
--
-- Added NOT VALID and validated in the next file. A plain ADD CONSTRAINT scans
-- every row under ACCESS EXCLUSIVE, so every write to deal and to lead would
-- wait for it. Neither scan can fail: the constraint each replaces held the
-- same rows against the same catalog, and only the update action changes.
SET LOCAL lock_timeout = '3s';

ALTER TABLE deal DROP CONSTRAINT deal_acquisition_source_fkey;
ALTER TABLE deal
	ADD CONSTRAINT deal_acquisition_source_fkey
		FOREIGN KEY (acquisition_source) REFERENCES deal_acquisition_source (key)
		ON UPDATE RESTRICT ON DELETE RESTRICT NOT VALID;

ALTER TABLE lead DROP CONSTRAINT lead_origin_fkey;
ALTER TABLE lead
	ADD CONSTRAINT lead_origin_fkey
		FOREIGN KEY (origin) REFERENCES lead_origin (key)
		ON UPDATE RESTRICT ON DELETE RESTRICT NOT VALID;

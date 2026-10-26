-- Restores the half rule and the subsumed foreign key, exactly as the catalog
-- held them, and takes the constraint comment with them.
SET LOCAL lock_timeout = '3s';

COMMENT ON CONSTRAINT stage_id_pipeline_unique ON stage IS NULL;

ALTER TABLE deal DROP CONSTRAINT deal_closed_fx;
ALTER TABLE deal
	ADD CONSTRAINT deal_closed_fx
		CHECK (status = 'open' OR amount_minor IS NULL
			OR fx_rate_to_base IS NOT NULL) NOT VALID;

ALTER TABLE deal
	ADD CONSTRAINT deal_stage_id_fkey FOREIGN KEY (stage_id)
		REFERENCES stage(id) ON DELETE RESTRICT NOT VALID;

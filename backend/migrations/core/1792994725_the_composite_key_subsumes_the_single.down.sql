-- Restores the subsumed key as the catalog held it, and takes the comment with
-- it. NOT VALID because deal is written and the pair it duplicates already
-- holds every row.
SET LOCAL lock_timeout = '3s';

COMMENT ON CONSTRAINT stage_id_pipeline_unique ON stage IS NULL;

ALTER TABLE deal
	ADD CONSTRAINT deal_stage_id_fkey FOREIGN KEY (stage_id)
		REFERENCES stage(id) ON DELETE RESTRICT NOT VALID;

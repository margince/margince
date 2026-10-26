-- deal carried three foreign keys into the pipeline model, and one was
-- redundant. deal_stage_in_pipeline (stage_id, pipeline_id) exists because
-- pipeline_id is a denormalised copy of stage.pipeline_id, and it stops a deal
-- whose pipeline_id names one pipeline while its stage_id belongs to another.
--
-- Both columns are NOT NULL, so there is no null-matching escape: a pair that
-- must exist in stage(id, pipeline_id) has a stage_id that exists in stage(id).
-- Deleting a stage is still refused, because a NO ACTION foreign key refuses
-- as RESTRICT does for a constraint that is not deferrable.
SET LOCAL lock_timeout = '3s';

ALTER TABLE deal DROP CONSTRAINT deal_stage_id_fkey;

-- Said on the constraint, because the migration that created it cannot be
-- edited and the catalog is where the next reader meets it. It reads as a
-- duplicate of stage_pkey and is not: a composite foreign key can only point
-- at a unique key over both of its columns.
COMMENT ON CONSTRAINT stage_id_pipeline_unique ON stage IS
	'Backs deal_stage_in_pipeline. Not a duplicate of stage_pkey: a composite foreign key needs a unique key over both columns.';

-- A closed deal freezes three things together: the rate, its date and the
-- converted amount. deal_closed_fx required only the rate, so a writer that
-- set the rate and not the converted amount passed every constraint. The deal
-- then drops out of every base-currency roll-up while looking correctly
-- closed, because a NULL amount_minor_base is skipped by SUM as an honest zero
-- rather than an invented rate.
--
-- The code already writes the three as one unit, in basecurrencyfreezewrite.go,
-- and clears the three on reopen. The database holds the same rule now, so a
-- later writer cannot be the exception.
--
-- THIS VALIDATION CAN FAIL, unlike most. The rule is one the data has never
-- been held to, so an installation carrying a closed deal with a frozen rate
-- and no converted amount fails the VALIDATE in the next file. That is the
-- right outcome: such a row is already missing from every base-currency total
-- while reading as closed, and a failure names it rather than hiding it.
--
-- deal_stage_id_fkey goes because the composite subsumes it. Both columns are
-- NOT NULL, so there is no null-matching escape: a (stage_id, pipeline_id)
-- that must exist in stage(id, pipeline_id) has a stage_id that exists in
-- stage(id). Deleting a stage is still refused, because a NO ACTION foreign
-- key refuses exactly as RESTRICT does for a non-deferrable constraint.
SET LOCAL lock_timeout = '3s';

ALTER TABLE deal DROP CONSTRAINT deal_stage_id_fkey;

ALTER TABLE deal DROP CONSTRAINT deal_closed_fx;
ALTER TABLE deal
	ADD CONSTRAINT deal_closed_fx
		CHECK (status = 'open' OR amount_minor IS NULL
			OR (fx_rate_to_base IS NOT NULL AND amount_minor_base IS NOT NULL)) NOT VALID;

-- Said on the constraint itself, because the migration that created it cannot
-- be edited and the next reader meets it in the catalog. It looks like a
-- duplicate of stage_pkey and is not: a composite foreign key can only point
-- at a unique key, and deal_stage_in_pipeline is what stops a deal naming one
-- pipeline while its stage belongs to another.
COMMENT ON CONSTRAINT stage_id_pipeline_unique ON stage IS
	'Backs deal_stage_in_pipeline. Not a duplicate of stage_pkey: a composite foreign key needs a unique key over both columns.';

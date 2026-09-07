-- Evidence is re-derivable from the records it cites; the two history columns
-- are additive and carry no data an older binary reads.
SET LOCAL lock_timeout = '5s';
ALTER TABLE deal_stage_history
  DROP CONSTRAINT deal_stage_history_reversal_fkey,
  DROP COLUMN reversal_of,
  DROP COLUMN approval_id;
DROP TABLE deal_stage_evidence;

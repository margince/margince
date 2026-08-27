-- Same bound as the up: dropping a column takes an ACCESS EXCLUSIVE lock on a
-- table the live product writes every morning.
SET LOCAL lock_timeout = '3s';

ALTER TABLE brief_item DROP CONSTRAINT IF EXISTS brief_item_finding_length;
ALTER TABLE brief_item DROP COLUMN IF EXISTS finding;

ALTER TABLE brief_run DROP CONSTRAINT IF EXISTS brief_run_narrative_length;
ALTER TABLE brief_run DROP CONSTRAINT IF EXISTS brief_run_narrative_needs_a_pass;
ALTER TABLE brief_run DROP COLUMN IF EXISTS annotated_at;
ALTER TABLE brief_run DROP COLUMN IF EXISTS narrative;

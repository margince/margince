SET LOCAL lock_timeout = '3s';
ALTER TABLE brief_run DROP CONSTRAINT IF EXISTS brief_run_factors_omitted_vocabulary;
ALTER TABLE brief_run DROP COLUMN IF EXISTS factors_omitted;

-- A row that was never changed carries the moment it was written, as on every
-- other table. A reader that asks whether it changed compares the two columns.
SET LOCAL lock_timeout = '3s';

UPDATE automation SET updated_at = created_at WHERE updated_at IS NULL;
ALTER TABLE automation
    ALTER COLUMN updated_at SET DEFAULT now(),
    ALTER COLUMN updated_at SET NOT NULL;
UPDATE voice_build SET updated_at = created_at WHERE updated_at IS NULL;
ALTER TABLE voice_build
    ALTER COLUMN updated_at SET DEFAULT now(),
    ALTER COLUMN updated_at SET NOT NULL;
UPDATE voice_corpus_source SET updated_at = created_at WHERE updated_at IS NULL;
ALTER TABLE voice_corpus_source
    ALTER COLUMN updated_at SET DEFAULT now(),
    ALTER COLUMN updated_at SET NOT NULL;
UPDATE voice_learning_signal SET updated_at = created_at WHERE updated_at IS NULL;
ALTER TABLE voice_learning_signal
    ALTER COLUMN updated_at SET DEFAULT now(),
    ALTER COLUMN updated_at SET NOT NULL;
UPDATE voice_profile SET updated_at = created_at WHERE updated_at IS NULL;
ALTER TABLE voice_profile
    ALTER COLUMN updated_at SET DEFAULT now(),
    ALTER COLUMN updated_at SET NOT NULL;
UPDATE voice_profile_delta SET updated_at = created_at WHERE updated_at IS NULL;
ALTER TABLE voice_profile_delta
    ALTER COLUMN updated_at SET DEFAULT now(),
    ALTER COLUMN updated_at SET NOT NULL;
UPDATE voice_profile_version SET updated_at = created_at WHERE updated_at IS NULL;
ALTER TABLE voice_profile_version
    ALTER COLUMN updated_at SET DEFAULT now(),
    ALTER COLUMN updated_at SET NOT NULL;

ALTER TABLE sales_goal_plan
    ALTER COLUMN created_at SET DEFAULT now(),
    ALTER COLUMN updated_at SET DEFAULT now();

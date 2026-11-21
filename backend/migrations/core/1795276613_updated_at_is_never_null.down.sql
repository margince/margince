-- The columns take a null again; the rows keep the moments they were given.
SET LOCAL lock_timeout = '3s';

ALTER TABLE automation
    ALTER COLUMN updated_at DROP NOT NULL,
    ALTER COLUMN updated_at DROP DEFAULT;
ALTER TABLE voice_build
    ALTER COLUMN updated_at DROP NOT NULL,
    ALTER COLUMN updated_at DROP DEFAULT;
ALTER TABLE voice_corpus_source
    ALTER COLUMN updated_at DROP NOT NULL,
    ALTER COLUMN updated_at DROP DEFAULT;
ALTER TABLE voice_learning_signal
    ALTER COLUMN updated_at DROP NOT NULL,
    ALTER COLUMN updated_at DROP DEFAULT;
ALTER TABLE voice_profile
    ALTER COLUMN updated_at DROP NOT NULL,
    ALTER COLUMN updated_at DROP DEFAULT;
ALTER TABLE voice_profile_delta
    ALTER COLUMN updated_at DROP NOT NULL,
    ALTER COLUMN updated_at DROP DEFAULT;
ALTER TABLE voice_profile_version
    ALTER COLUMN updated_at DROP NOT NULL,
    ALTER COLUMN updated_at DROP DEFAULT;
ALTER TABLE sales_goal_plan
    ALTER COLUMN created_at DROP DEFAULT,
    ALTER COLUMN updated_at DROP DEFAULT;

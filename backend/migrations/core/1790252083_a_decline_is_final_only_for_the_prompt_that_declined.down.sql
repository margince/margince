SET LOCAL lock_timeout = '5s';

ALTER TABLE activity
    DROP COLUMN IF EXISTS owed_verdict_declined_ruleset,
    DROP COLUMN IF EXISTS capture_label_declined_ruleset;

-- Same bound as the up: dropping a column takes an ACCESS EXCLUSIVE lock on a
-- table the live product writes every Monday.
SET LOCAL lock_timeout = '3s';

ALTER TABLE weekly_review DROP CONSTRAINT IF EXISTS weekly_review_narrative_length;
ALTER TABLE weekly_review DROP CONSTRAINT IF EXISTS weekly_review_narrative_needs_a_pass;
ALTER TABLE weekly_review DROP COLUMN IF EXISTS narrated_at;
ALTER TABLE weekly_review DROP COLUMN IF EXISTS narrative;

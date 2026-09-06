SET LOCAL lock_timeout = '3s';

DROP TABLE IF EXISTS weekly_review_learning_citation;
DROP TABLE IF EXISTS weekly_review_learning;

ALTER TABLE weekly_review
    DROP CONSTRAINT IF EXISTS weekly_review_learned_at_matches_state,
    DROP CONSTRAINT IF EXISTS weekly_review_learnings_state_check;

ALTER TABLE weekly_review
    DROP COLUMN IF EXISTS learned_at,
    DROP COLUMN IF EXISTS learnings_state;

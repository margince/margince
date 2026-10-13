SET LOCAL lock_timeout = '3s';

DROP TABLE IF EXISTS team_weekly_review_rep_funnel;
DROP TABLE IF EXISTS team_weekly_review_coverage;
ALTER TABLE team_weekly_review DROP CONSTRAINT IF EXISTS team_weekly_review_quality_tallies;
ALTER TABLE team_weekly_review
    DROP COLUMN IF EXISTS proposals_accepted,
    DROP COLUMN IF EXISTS proposals_rejected,
    DROP COLUMN IF EXISTS auto_change_reversals,
    DROP COLUMN IF EXISTS summary;
DROP TABLE IF EXISTS team_weekly_review_member;
DROP TABLE IF EXISTS team_weekly_review_rep_pattern;

-- The file after this one narrowed the focus vocabulary back NOT VALID on its
-- way down; validating it here, in a later transaction, restores the old schema.
ALTER TABLE team_weekly_review_rep VALIDATE CONSTRAINT team_weekly_review_rep_focus_kind_check;

ALTER TABLE weekly_review_scorecard DROP CONSTRAINT IF EXISTS weekly_review_scorecard_provisional_fits;
ALTER TABLE weekly_review_scorecard DROP COLUMN IF EXISTS deal_close_date_provisional;

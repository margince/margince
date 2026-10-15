-- The three constraints the file before this one added NOT VALID are validated
-- here, in their own transaction, under a lock readers and writers pass.
SET LOCAL lock_timeout = '3s';

ALTER TABLE weekly_review_scorecard VALIDATE CONSTRAINT weekly_review_scorecard_provisional_fits;
ALTER TABLE team_weekly_review_rep VALIDATE CONSTRAINT team_weekly_review_rep_focus_kind_check;
ALTER TABLE team_weekly_review VALIDATE CONSTRAINT team_weekly_review_quality_tallies;

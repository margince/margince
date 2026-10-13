-- Validation itself is not revertible. The focus vocabulary is narrowed back
-- here rather than in the file before this one, so that file's down can
-- validate it in a transaction of its own and leave the old schema whole.
SET LOCAL lock_timeout = '3s';

UPDATE team_weekly_review_rep SET focus_kind = 'deals_at_risk'
 WHERE focus_kind IN ('no_next_step_coverage', 'close_dates_provisional',
                      'forecast_downgrades', 'stage_regressions');
ALTER TABLE team_weekly_review_rep DROP CONSTRAINT team_weekly_review_rep_focus_kind_check;
ALTER TABLE team_weekly_review_rep ADD CONSTRAINT team_weekly_review_rep_focus_kind_check
    CHECK (focus_kind IN ('help_requested', 'leads_breached', 'commitments_missed',
        'meetings_without_next_step', 'deals_at_risk', 'strong_week', 'quiet_week')) NOT VALID;

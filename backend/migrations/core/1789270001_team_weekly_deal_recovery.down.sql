UPDATE team_weekly_review_rep SET focus_kind = 'quiet_week' WHERE focus_kind = 'deals_at_risk';
ALTER TABLE team_weekly_review_rep DROP CONSTRAINT team_weekly_review_rep_focus_kind_check;
ALTER TABLE team_weekly_review_rep ADD CONSTRAINT team_weekly_review_rep_focus_kind_check CHECK (focus_kind IN ('help_requested', 'leads_breached', 'commitments_missed', 'meetings_without_next_step', 'strong_week', 'quiet_week'));

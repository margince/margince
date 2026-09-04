-- Bounded so a migration never queues behind an open transaction forever:
-- without it, one long-running reader stalls every write to team_weekly_review
-- for as long as this is willing to wait.
SET LOCAL lock_timeout = '3s';

ALTER TABLE team_weekly_review
    DROP CONSTRAINT team_weekly_review_counts_are_tallies;

ALTER TABLE team_weekly_review
    ADD CONSTRAINT team_weekly_review_counts_are_tallies CHECK (
        reps_counted >= 0 AND reps_unread >= 0
        AND deals_won >= 0 AND deals_lost >= 0
        AND leads_routed >= 0 AND leads_answered_in_target >= 0 AND leads_breached >= 0
        AND meetings_held >= 0 AND meetings_with_next_step >= 0
        AND commitments_due >= 0 AND commitments_kept >= 0
        AND (pipeline_created_minor IS NULL OR pipeline_created_minor >= 0)
        AND (pipeline_won_minor IS NULL OR pipeline_won_minor >= 0)
        AND (pipeline_lost_minor IS NULL OR pipeline_lost_minor >= 0));

ALTER TABLE team_weekly_review DROP COLUMN deals_moved;

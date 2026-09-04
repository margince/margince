-- The team's week counts the deals that MOVED, not only the ones that closed.
--
-- Every member's weekly_review has carried deals_moved since the review shipped
-- — deals that changed stage without closing. The team snapshot summed won and
-- lost and dropped it, so a team that advanced eleven deals and closed none
-- read as a team that did nothing at all. That is the week most teams have.
--
-- A COUNT, not money. Advancing is a stage fact and slipping is a date fact;
-- neither has an amount that means anything. A deal moving from Proposal to
-- Negotiation did not change price, so a "value advanced" figure would be the
-- deal's whole worth counted again in a second column beside the pipeline it
-- is already in.
--
-- Additive and nullable-free: the column defaults to zero, so snapshots written
-- before this migration read as zero rather than unknown. That is honest here
-- in a way it would not be for money — a count of zero is a count, while a sum
-- of zero over deals nobody could price is a claim about a week nobody
-- measured. The existing rows are weeks whose figure was never computed, and
-- the next assembly overwrites nothing: a snapshot is written once.
-- Bounded so a migration never queues behind an open transaction forever:
-- without it, one long-running reader stalls every write to team_weekly_review
-- for as long as this is willing to wait.
SET LOCAL lock_timeout = '3s';

ALTER TABLE team_weekly_review
    ADD COLUMN deals_moved int DEFAULT 0 NOT NULL;

ALTER TABLE team_weekly_review
    DROP CONSTRAINT team_weekly_review_counts_are_tallies;

ALTER TABLE team_weekly_review
    ADD CONSTRAINT team_weekly_review_counts_are_tallies CHECK (
        reps_counted >= 0 AND reps_unread >= 0
        AND deals_won >= 0 AND deals_lost >= 0 AND deals_moved >= 0
        AND leads_routed >= 0 AND leads_answered_in_target >= 0 AND leads_breached >= 0
        AND meetings_held >= 0 AND meetings_with_next_step >= 0
        AND commitments_due >= 0 AND commitments_kept >= 0
        AND (pipeline_created_minor IS NULL OR pipeline_created_minor >= 0)
        AND (pipeline_won_minor IS NULL OR pipeline_won_minor >= 0)
        AND (pipeline_lost_minor IS NULL OR pipeline_lost_minor >= 0));

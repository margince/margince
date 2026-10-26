-- The number of leads the week's response figures were read from.
--
-- A frozen scorecard carried how many of the week's leads got a first response
-- by the close (lead_answered_in_target, late answers included) and how many
-- breached, but not how many arrived. Without the cohort a reader could not tell
-- a week where every lead was answered from one where half were never answered
-- at all, and the panel said "All answered in target" whenever no breach was
-- recorded, which a lead nobody answered and nobody stamped also satisfies.
--
-- Nullable and outside weekly_review_scorecard_lead_block_shape, for the reason
-- deal_unreconstructible is: every scorecard frozen before this column existed
-- has a lead block and no cohort, and the CHECK would refuse those rows. NULL
-- therefore means "this week was frozen before its cohort was recorded", which
-- a reader must show as unknown and never as zero.
-- Bounded, because ALTER TABLE takes an ACCESS EXCLUSIVE lock and a weekly
-- scorecard is written by a job that runs while people are reading.
SET LOCAL lock_timeout = '5s';

ALTER TABLE weekly_review_scorecard
    ADD COLUMN lead_arrived integer;

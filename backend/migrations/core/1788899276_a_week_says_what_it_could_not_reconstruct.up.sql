-- The deals whose week could not be reconstructed.
--
-- The deal block's population counts (open, with a next step, multi-threaded,
-- close date sound) describe the pipeline as the week CLOSED, rebuilt by
-- walking the audit spine backwards from the closing instant. A deal whose
-- post-cutoff changes sit behind an erasure cannot be walked: the spine is
-- append-only, so the images are still there, and reading them would put back
-- onto a rep's review exactly what a scrub certified destroyed.
--
-- Such a deal is left out of every count above and TALLIED HERE, because a
-- reader who is not told takes a smaller open count for the whole truth. Zero
-- is the ordinary answer and means the counts beside it are complete.
--
-- Left OUT of weekly_review_scorecard_deal_block_shape deliberately, for the
-- reason deal_median_days_in_stage is: that CHECK binds columns which are
-- present exactly when the block is, and this one is NULL on every scorecard
-- frozen before the reconstruction existed. Adding it there would refuse rows
-- already written and shipped. NULL therefore means "this week was scored
-- before the question was asked", which a reader must not read as zero — those
-- reviews' population counts are the current-state figures they always were,
-- and nothing here can retroactively make them week-end facts.
-- Bounded, because ALTER TABLE takes an ACCESS EXCLUSIVE lock and a weekly
-- scorecard is written by a job that runs while people are reading. Waiting
-- unbounded behind one long reader would queue every writer behind this.
SET LOCAL lock_timeout = '5s';

ALTER TABLE weekly_review_scorecard
    ADD COLUMN deal_unreconstructible integer;

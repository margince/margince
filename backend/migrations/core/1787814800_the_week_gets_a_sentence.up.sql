-- One or two sentences over the week's counts, for the rep who reads them on
-- Monday.
--
-- The counts and the deal lines are the retrospective; this is what a colleague
-- would say about them. It is written by a model, so the columns are shaped for
-- a model NOT being there: a rep with no bound lane, an exhausted budget or a
-- provider outage still gets the whole review, and the screen has to be able to
-- tell that apart from a week nobody had anything to say about.
--
-- NULLABLE, never NOT NULL DEFAULT ''. A week with no narrative genuinely has
-- none, and an empty string would make "the model did not run" and "the model
-- had nothing to add" the same value.
--
-- TWO COLUMNS, and narrated_at is not derivable from the sentence being
-- present: a pass that ran and honestly found the week unremarkable writes the
-- stamp with no sentence. Collapsing them would make an honest quiet week look
-- like a broken one — the same distinction brief_run carries, for the same
-- reason.
SET LOCAL lock_timeout = '3s';

ALTER TABLE weekly_review ADD COLUMN narrative text;
ALTER TABLE weekly_review ADD COLUMN narrated_at timestamptz;

ALTER TABLE weekly_review
    ADD CONSTRAINT weekly_review_narrative_needs_a_pass
    CHECK (narrative IS NULL OR narrated_at IS NOT NULL);

-- Bounded so one model's runaway output cannot become a row nothing can
-- render. Generous for two sentences, far below anything that breaks a panel.
ALTER TABLE weekly_review
    ADD CONSTRAINT weekly_review_narrative_length
    CHECK (narrative IS NULL OR char_length(narrative) <= 600);

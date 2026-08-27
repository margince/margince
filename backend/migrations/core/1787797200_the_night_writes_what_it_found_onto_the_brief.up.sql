-- The overnight agent's findings get a place to live.
--
-- The morning_brief agent's goal already asks for "why it is on the list, what
-- changed recently, and one recommended next move — every claim grounded in a
-- record you actually read, citing its id" (runner/catalog.go). It has been
-- asked that since it shipped. There has been nowhere to put the answer, so it
-- has been going into agent_run.result, which nothing renders: the rep sees a
-- ranked queue of deals with meters and no sentence saying why any of them is
-- there.
--
-- TWO PLACES, because they are two different claims. brief_run.narrative is
-- one sentence about the NIGHT ("2 replies, 1 deal went quiet, 1 promise due
-- today") and belongs to the run. The per-item finding is about ONE deal and
-- belongs beside the rank it explains.
--
-- The columns are nullable and stay that way. A run assembled without a model,
-- without a grant, or with an exhausted budget has no narrative and must say
-- so — a NOT NULL default of empty string would make "the model did not run"
-- and "the model had nothing to say" the same value, and the screen cannot
-- then tell an honest silence from a broken one.
--
-- Bounded, because every statement below blocks writers on tables this
-- migration did not create.
SET LOCAL lock_timeout = '3s';

ALTER TABLE brief_run ADD COLUMN narrative text;
ALTER TABLE brief_run ADD COLUMN annotated_at timestamptz;

-- annotated_at is not decoration and not derivable from narrative being
-- non-null: a pass that ran and honestly found nothing worth saying writes a
-- stamp with no sentence, and the screen needs to tell that apart from a pass
-- that never ran at all.
ALTER TABLE brief_run
    ADD CONSTRAINT brief_run_narrative_needs_a_pass
    CHECK (narrative IS NULL OR annotated_at IS NOT NULL);

-- Bounded so one model's runaway output cannot become a row nothing can
-- render. The ceiling is generous for a sentence and far below anything that
-- would break a card.
ALTER TABLE brief_run
    ADD CONSTRAINT brief_run_narrative_length
    CHECK (narrative IS NULL OR char_length(narrative) <= 600);

ALTER TABLE brief_item ADD COLUMN finding text;

ALTER TABLE brief_item
    ADD CONSTRAINT brief_item_finding_length
    CHECK (finding IS NULL OR char_length(finding) <= 600);

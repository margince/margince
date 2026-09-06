-- What could go wrong with the week, and whether there is room for it.
--
-- The plan already carries what a rep MEANS to do. These two say what they
-- expect to get in the way, and what their calendar leaves them — the half of a
-- plan a lead actually reads, because a list of five commitments with no room
-- to do them is a plan that has already failed and nobody has said so.
--
-- Both are prose the rep writes, not derived figures. Capacity is COUNTED
-- elsewhere (booked meetings and tasks due in next week's window, through a
-- seam) and this column is what the rep says ABOUT that count — "two days at
-- the conference" is a fact no query has.
--
-- Nullable, and NULL is not an empty string: a rep who has not written their
-- risks has said nothing, and one who cleared the field has said there are
-- none. The surface draws those differently.
SET LOCAL lock_timeout = '3s';

ALTER TABLE weekly_plan
    ADD COLUMN risks text,
    ADD COLUMN capacity_note text;

-- 2000, the same ceiling every other prose column on a plan carries
-- (weekly_plan_commitment's help_requested and manager_response) and the same
-- one weeklyplan.proseBound applies at the seam.
--
-- Held HERE as well as in Go because a column with no ceiling is one a second
-- writer can fill without noticing. The two numbers must agree, and
-- TestEveryPlanProseColumnMatchesItsGoBound fails in both directions if they
-- drift.
ALTER TABLE weekly_plan
    ADD CONSTRAINT weekly_plan_risks_bounded
    CHECK (risks IS NULL OR length(risks) <= 2000);

ALTER TABLE weekly_plan
    ADD CONSTRAINT weekly_plan_capacity_note_bounded
    CHECK (capacity_note IS NULL OR length(capacity_note) <= 2000);

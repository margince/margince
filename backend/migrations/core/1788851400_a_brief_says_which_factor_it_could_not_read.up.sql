-- A brief names the ranking factor it was not allowed to read.
--
-- The queue is ordered by several factors, one of which is the warmth of the
-- deal's stakeholders. A caller with no `relationship` edge grant runs no
-- stakeholder read at all, so that factor floored for every deal and the queue
-- came back ordered LOWER than it should be, with nothing saying why. A rank
-- computed from a withheld input is a wrong answer, not an incomplete one.
--
-- Stored with the run rather than resolved on read, for the same reason
-- revenue_norm_currency is: the grant can be given or taken away afterwards,
-- and a queue ranked without warmth must not later be read as one that had it.
--
-- An array rather than a boolean because it answers "which", and the queue has
-- more than one factor that could become unreadable. Empty is the ordinary
-- answer and is distinct from absent: every run written from here on says
-- something, and a run stored before this column existed reads as empty — those
-- age out within a day, since a rep has exactly one run per local day.
SET LOCAL lock_timeout = '3s';
ALTER TABLE brief_run
    ADD COLUMN IF NOT EXISTS factors_omitted text[] DEFAULT '{}'::text[] NOT NULL;

-- The vocabulary is closed: a factor nothing can render is a factor a client
-- silently drops, which is the failure this column exists to end.
ALTER TABLE brief_run
    DROP CONSTRAINT IF EXISTS brief_run_factors_omitted_vocabulary;
ALTER TABLE brief_run
    ADD CONSTRAINT brief_run_factors_omitted_vocabulary
        CHECK (factors_omitted <@ ARRAY['warmth']::text[]);

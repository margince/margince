-- One task per subject per cycle, instead of one task per exception.
--
-- The nightly check raises exceptions and a human resolves them, which works.
-- What it cannot do is give somebody ONE thing to act on: a deal with four
-- problems produces four tasks, and a rep clearing them clears the same deal
-- four times. Bundling is what turns a list of findings into a piece of work.
--
-- THE UNIQUE CONSTRAINT IS THE IDEMPOTENCY. The minting path is
-- `INSERT ... ON CONFLICT DO NOTHING RETURNING`, so a second pass over the same
-- cycle mints nothing and says so by returning no row. A dedupe wrapper in
-- Redis would answer the same question until the moment it is flushed, and then
-- answer it wrong — quietly, by minting a duplicate task somebody has to notice.
SET LOCAL lock_timeout = '3s';

-- A cycle is one pass of assurance over a scope: the window that groups
-- findings into work.
--
-- It has no workspace_id, and that is this tree's shape rather than an omission
-- — tenancy is per-database and no table carries one.
CREATE TABLE IF NOT EXISTS assurance_cycle (
    id uuid PRIMARY KEY DEFAULT uuidv7(),
    -- What this cycle covers. Free text rather than an enum: a scope is an
    -- operator's description of a pass ("Q3 pipeline", "EMEA offers"), and a
    -- closed vocabulary would make the product decide what somebody may audit.
    scope text NOT NULL,
    opened_by text NOT NULL,
    opened_at timestamptz NOT NULL DEFAULT now(),
    -- NULL while the cycle is open. A closed cycle mints no new tasks: its
    -- window is over, and a finding after it belongs to the next pass.
    closed_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT assurance_cycle_scope_present CHECK (length(btrim(scope)) > 0),
    -- A cycle cannot close before it opened. Cheap to state and the kind of
    -- inversion an import or a hand-written UPDATE produces.
    CONSTRAINT assurance_cycle_closes_after_opening CHECK (
        closed_at IS NULL OR closed_at >= opened_at)
);

-- Only ONE cycle may be open per scope at a time.
--
-- Two open cycles over the same scope would each mint their own task for the
-- same deal, which is the duplication this table exists to prevent — arriving
-- one level up, where the unique constraint below cannot see it.
CREATE UNIQUE INDEX IF NOT EXISTS assurance_cycle_one_open_per_scope
    ON assurance_cycle (scope) WHERE closed_at IS NULL;

-- What one exception contributed to one cycle's task.
--
-- The exception is the finding; the task is the work. Several exceptions about
-- one deal join the SAME task row through their shared (cycle, subject), and
-- this table is what records which findings that task answers for.
CREATE TABLE IF NOT EXISTS assurance_task_item (
    id uuid PRIMARY KEY DEFAULT uuidv7(),
    cycle_id uuid NOT NULL REFERENCES assurance_cycle(id) ON DELETE CASCADE,
    exception_id uuid NOT NULL REFERENCES assurance_exception(id) ON DELETE CASCADE,
    -- The task itself is an activity, minted through the same door REST
    -- CreateTask and MCP create_task use — there is no second task type here,
    -- and a bundled task is one a rep sees on their own list like any other.
    task_activity_id uuid NOT NULL REFERENCES activity(id) ON DELETE CASCADE,
    -- What the SUBJECT was when this item was minted. Carried rather than joined
    -- because it is the bundling key: a task groups a cycle's findings about one
    -- subject, and the unique index below has to see the pair.
    subject_kind text NOT NULL,
    subject_id uuid NOT NULL,
    state text NOT NULL DEFAULT 'open',
    created_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT assurance_task_item_state CHECK (state IN ('open', 'resolved', 'dismissed')),
    CONSTRAINT assurance_task_item_subject_kind CHECK (
        subject_kind IN ('deal', 'signal', 'offer', 'contract')),
    -- ONE item per exception per cycle. This is the idempotency the minting path
    -- rests on: a re-run of the same cycle conflicts here and inserts nothing,
    -- which is a property of the data rather than of a cache that was warm.
    CONSTRAINT assurance_task_item_once UNIQUE (exception_id, cycle_id)
);

-- ONE task per subject per cycle — the bundling itself.
--
-- An EXCLUSION constraint rather than a unique index, because the rule is not
-- "one row per subject" — several findings about one deal is exactly what a
-- bundle IS, and a unique index over (cycle, subject) forbids the second one.
-- What must not vary is the TASK those rows point at.
--
-- The first draft of this file indexed (cycle, subject, task) and refused a
-- second finding about the same deal, which is the feature. The test that mints
-- three findings for one deal is what caught it; nothing else would have,
-- because a bundle of one looks exactly like a bundle that works.
ALTER TABLE assurance_task_item
    ADD CONSTRAINT assurance_task_item_one_task_per_subject
    EXCLUDE USING gist (
        cycle_id WITH =, subject_kind WITH =, subject_id WITH =,
        task_activity_id WITH <>
    );

-- What a rep is being asked to do, and what is still open in a cycle.
CREATE INDEX IF NOT EXISTS assurance_task_item_by_task
    ON assurance_task_item (task_activity_id);
CREATE INDEX IF NOT EXISTS assurance_task_item_open_in_cycle
    ON assurance_task_item (cycle_id, subject_kind, subject_id)
    WHERE state = 'open';

COMMENT ON TABLE assurance_cycle IS
    'One pass of assurance over a scope. Findings inside it bundle into one task per subject.';
COMMENT ON TABLE assurance_task_item IS
    'What one exception contributed to one cycle''s task. The unique constraint on (exception_id, cycle_id) is the minting idempotency.';

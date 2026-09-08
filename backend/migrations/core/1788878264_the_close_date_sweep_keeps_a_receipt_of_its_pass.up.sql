-- The close-date sweep could not say what it had checked, so it checked the
-- same deals every night.
--
-- Its candidate query selects open deals with a missing, provisional or
-- near-term date, ORDER BY created_at, id LIMIT 200 — with no cursor and no
-- record of a pass. Two things follow, and the second is the damage.
--
-- A corrected deal STAYS eligible: the sweep writes a provisional date, and
-- provisional is one of the three conditions the pre-filter admits. So the same
-- oldest 200 rows re-qualify every night, the LIMIT cuts at the same place, and
-- a workspace with more than 200 eligible deals never reaches number 201. An
-- overdue deal sitting at position 300 is not assessed late; it is not assessed
-- at all. The sweep's own comment claims a backlog "drains over successive
-- nights", which only holds if inspected candidates leave the population.
--
-- Nothing recorded this. Assurance keeps assurance_run with its eligible counts
-- and per-source coverage, so a partial assurance pass is visible; the
-- close-date sweep kept nothing, so its starvation was unobservable from the
-- outside. A receipt is what turns "the job completed" into "these deals were
-- checked and those were not".
--
-- Two tables, because a run and its membership answer different questions and
-- have different lifetimes.
--
-- close_date_run is the pass: when it started, the day it judged against, how
-- far it got, and whether it finished. The counters are derived from membership
-- rather than trusted on their own — they exist so a reader does not have to
-- aggregate the member table to render a coverage line.
--
-- close_date_run_member is the FROZEN ELIGIBLE SET, captured at run start. This
-- is what makes the pass resumable and honest at once: membership is decided
-- once, so a deal that changes, closes or is archived mid-pass keeps its place
-- in the ledger and gets an explicit outcome, and a deal created after the
-- freeze belongs to the next run rather than silently joining this one. The
-- cursor walks the member rows, not the live deal table, so a re-read cannot
-- return a different population than the one being counted against.
--
-- The 5-minute worker timeout is why the cursor is durable rather than held in
-- memory: a pass interrupted mid-way resumes at the checkpoint instead of
-- starting over at the first row, which is what made a retry re-read page one.

SET LOCAL lock_timeout = '5s';

CREATE TABLE close_date_run (
    id uuid PRIMARY KEY DEFAULT uuidv7(),
    -- The workspace-zone day this pass judges "overdue" against. Frozen at
    -- start so a run crossing local midnight keeps one answer.
    as_of date NOT NULL,
    started_at timestamptz NOT NULL DEFAULT now(),
    finished_at timestamptz,
    status text NOT NULL DEFAULT 'running',
    -- Size of the frozen set, and how much of it reached a terminal outcome.
    -- remaining is eligible - checked; it is not stored, because a stored
    -- difference is a third number that can disagree with the other two.
    eligible integer NOT NULL DEFAULT 0,
    checked integer NOT NULL DEFAULT 0,
    corrected integer NOT NULL DEFAULT 0,
    staged integer NOT NULL DEFAULT 0,
    -- The keyset position the pass has durably passed, in the member table's
    -- own order. NULL means it has not started walking.
    cursor_created_at timestamptz,
    cursor_id uuid,
    captured_by text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    version bigint NOT NULL DEFAULT 1,
    CONSTRAINT close_date_run_status_check
        CHECK (status = ANY (ARRAY['running'::text, 'complete'::text, 'incomplete'::text])),
    CONSTRAINT close_date_run_counts_are_tallies
        CHECK (eligible >= 0 AND checked >= 0 AND corrected >= 0 AND staged >= 0),
    -- A finished run has a finishing time and a running one does not, so the
    -- two cannot drift apart into a run that is complete but still open.
    CONSTRAINT close_date_run_finish_matches_status
        CHECK ((status = 'running') = (finished_at IS NULL)),
    -- The cursor is a keyset pair: both halves or neither.
    CONSTRAINT close_date_run_cursor_is_whole
        CHECK ((cursor_created_at IS NULL) = (cursor_id IS NULL))
);

CREATE TABLE close_date_run_member (
    run_id uuid NOT NULL REFERENCES close_date_run(id) ON DELETE CASCADE,
    deal_id uuid NOT NULL REFERENCES deal(id) ON DELETE CASCADE,
    -- The deal's keyset position, copied at freeze time. Copied rather than
    -- joined because the pass must walk a stable order even if the deal's own
    -- created_at were ever corrected underneath it.
    deal_created_at timestamptz NOT NULL,
    -- pending until the pass reaches it; then exactly one terminal word.
    --   checked  — assessed, nothing to do
    --   changed  — a correction committed
    --   staged   — a decision was raised for a human
    --   skipped  — no longer applicable (closed or archived since the freeze)
    --   failed   — the assessment or write errored; the run is incomplete
    outcome text NOT NULL DEFAULT 'pending',
    settled_at timestamptz,
    PRIMARY KEY (run_id, deal_id),
    CONSTRAINT close_date_run_member_outcome_check
        CHECK (outcome = ANY (ARRAY['pending'::text, 'checked'::text, 'changed'::text,
                                    'staged'::text, 'skipped'::text, 'failed'::text])),
    CONSTRAINT close_date_run_member_settled_matches_outcome
        CHECK ((outcome = 'pending') = (settled_at IS NULL))
);

-- The pass's own read: the next unsettled members in keyset order.
CREATE INDEX close_date_run_member_walk
    ON close_date_run_member (run_id, deal_created_at, deal_id)
    WHERE outcome = 'pending';

-- Resuming looks for tonight's unfinished run; reporting reads the latest.
CREATE INDEX close_date_run_open
    ON close_date_run (as_of DESC) WHERE status = 'running';

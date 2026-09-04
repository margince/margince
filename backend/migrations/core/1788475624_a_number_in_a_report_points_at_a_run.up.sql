-- The result a report block points at.
--
-- A report sentence does not carry a number. It carries a POINTER to a cell of
-- a result that really ran, and the number is dereferenced when the report is
-- read. That is the whole reason this table exists: without a saved result the
-- only place a figure in a document can come from is whoever composed the
-- sentence, and for a generated report that is a model typing a plausible
-- number nobody can trace.
--
-- So a row here is immutable. There is no version column and nothing updates
-- it: a re-run is a NEW run with a new id, and a block that should show fresher
-- numbers is re-pointed rather than rewritten underneath its reader. A mutable
-- result cell would mean a report whose sentences silently change meaning after
-- they were approved.
--
-- What it does NOT do is let one reader see another reader's answer. The rows
-- are stored exactly as they were served, already narrowed and already floored
-- for the person who asked -- and that means they may not be replayed for
-- somebody else. `asked_by` records whose answer this is, and the reader re-runs
-- the question under its own grants rather than trusting the stored cells. The
-- stored copy is what makes a number CITABLE and comparable over time, not a
-- cache that skips a permission check.
SET LOCAL lock_timeout = '5s';

CREATE TABLE report_run (
    id uuid DEFAULT uuidv7() NOT NULL,

    -- The question, exactly as it was asked. Kept so the run can be re-asked
    -- under a different reader's grants, and so two runs can be told apart by
    -- what they asked rather than by when they ran.
    query jsonb NOT NULL,

    -- The answer, exactly as it was served: the column order, the rows, and
    -- the two flags a reader needs to interpret them. Stored together in one
    -- document because they are one answer -- a row set separated from the
    -- withheld flag that qualifies it is a number without its caveat.
    -- Named result_columns/result_rows, not columns/rows: ROWS is a reserved
    -- word in Postgres and an unquoted one is a syntax error at every call
    -- site, which is a trap for the next person writing a statement by hand.
    result_columns jsonb NOT NULL,
    result_rows jsonb NOT NULL,
    withheld boolean NOT NULL,
    total_safe boolean NOT NULL,

    -- The vocabulary the query was compiled against. Two runs are only
    -- comparable if they were asked in the same language, and the schema
    -- narrows per reader, so this is not a constant.
    schema_version text NOT NULL,

    -- Whose answer this is. NOT a convenience column: it is what stops the
    -- stored rows being served to a second reader, whose grants may narrow
    -- the population differently.
    asked_by uuid NOT NULL,

    -- The floor that judged it. A run served under a floor of 5 and a run
    -- served under a floor of 1 make different promises about what is missing,
    -- and a reader comparing them needs to be told.
    group_floor integer NOT NULL,

    captured_by text NOT NULL,
    created_at timestamptz DEFAULT now() NOT NULL,

    CONSTRAINT report_run_pkey PRIMARY KEY (id),
    -- An answer is a LIST of rows and a LIST of columns. A jsonb object where
    -- an array belongs is a shape the reader would index into and get null
    -- from, which reads exactly like a withheld cell.
    CONSTRAINT report_run_result_rows_is_a_list CHECK (jsonb_typeof(result_rows) = 'array'),
    CONSTRAINT report_run_result_columns_is_a_list CHECK (jsonb_typeof(result_columns) = 'array'),
    -- A floor of zero is no floor. Storing one would misdescribe an answer
    -- that was in fact served unfloored.
    CONSTRAINT report_run_floor_is_not_negative CHECK (group_floor >= 0)
);

ALTER TABLE report_run
    ADD CONSTRAINT report_run_asked_by_fkey FOREIGN KEY (asked_by)
    REFERENCES app_user(id) ON DELETE CASCADE;

-- A departed employee's saved answers go with them, the same rule their shares
-- keep. The alternative is a deactivation that fails because somebody once ran
-- a report, and answers narrowed to their grants outliving their seat.

-- The listing a reader makes: their own runs, newest first.
CREATE INDEX idx_report_run_asker ON report_run (asked_by, created_at DESC);

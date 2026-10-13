SET LOCAL lock_timeout = '3s';

-- An import run is a saved session: a draft keeps the user's choices between
-- requests, and the file it was read from is released once no step needs it.
ALTER TABLE import_run
    ADD COLUMN version int NOT NULL DEFAULT 1,
    ADD COLUMN draft_step text,
    ADD COLUMN profile jsonb,
    ADD COLUMN approved_by text,
    ADD COLUMN approved_at timestamptz,
    -- Generated, so an older writer that only knows `mapping` still files the name.
    ADD COLUMN source_name text GENERATED ALWAYS AS (coalesce(mapping->>'source_name', '')) STORED,
    ADD COLUMN source_released_at timestamptz,
    ADD COLUMN superseded_by uuid;

ALTER TABLE import_run
    DROP CONSTRAINT import_run_status_check,
    ADD CONSTRAINT import_run_status_check CHECK (status IN (
        'pending', 'validating', 'awaiting_approval', 'running', 'complete',
        'failed', 'undoing', 'undone', 'cancelled')) NOT VALID,
    ADD CONSTRAINT import_run_draft_step_check CHECK (draft_step IN ('files', 'map')) NOT VALID;

-- A test run that stopped wrote nothing, so it ends cancelled and frees its file.
-- The text is migration.CauseValidationStopped.
UPDATE import_run
   SET status = 'cancelled', source_released_at = now()
 WHERE status = 'failed'
   AND error = 'the test run stopped on an error; upload the file again to start a new import';

-- Every run that reached a commit was approved, and a resume asks for that stamp.
UPDATE import_run
   SET approved_at = updated_at
 WHERE status IN ('running', 'complete', 'undoing', 'undone', 'failed');

-- One active run per source name: a run that writes keeps the name, else the
-- newest report does, and every other report of it is superseded by that run.
-- The error is migration.CauseSuperseded.
WITH ranked AS (
    SELECT id,
           first_value(id) OVER named AS keeper,
           row_number() OVER named AS rank
      FROM import_run
     WHERE connector = 'csv' AND source_name <> ''
       AND status IN ('validating', 'awaiting_approval', 'running', 'undoing')
    WINDOW named AS (PARTITION BY source_name
                     ORDER BY status IN ('running', 'undoing') DESC, created_at DESC, id DESC)
)
UPDATE import_run r
   SET status = 'cancelled', superseded_by = ranked.keeper, source_released_at = now(),
       error = 'a newer test run of this source replaced this one'
  FROM ranked
 WHERE r.id = ranked.id AND ranked.rank > 1
   AND r.status IN ('validating', 'awaiting_approval');

-- One unreleased run per upload. The run that still needs the file keeps it: a
-- commit or undo in flight or resumable first, else the newest. A released run
-- that had written nothing is superseded; a finished one keeps its status.
WITH ranked AS (
    SELECT id,
           first_value(id) OVER upload AS keeper,
           row_number() OVER upload AS rank
      FROM import_run
     WHERE connector = 'csv' AND source_released_at IS NULL
    WINDOW upload AS (PARTITION BY source_ref
                      ORDER BY status IN ('running', 'failed', 'undoing') DESC, created_at DESC, id DESC)
)
UPDATE import_run r
   SET source_released_at = now(),
       status = CASE WHEN r.status IN ('pending', 'validating', 'awaiting_approval')
                     THEN 'cancelled' ELSE r.status END,
       superseded_by = CASE WHEN r.status IN ('pending', 'validating', 'awaiting_approval')
                            THEN ranked.keeper ELSE r.superseded_by END,
       error = CASE WHEN r.status IN ('pending', 'validating', 'awaiting_approval')
                    THEN 'a newer test run of this source replaced this one' ELSE r.error END
  FROM ranked
 WHERE r.id = ranked.id AND ranked.rank > 1;

-- Built plainly in the transaction that deduplicated, so no writer can add a
-- duplicate between the dedupe and the index. The table holds one row per import.
CREATE UNIQUE INDEX import_run_one_active_per_source_name ON import_run (source_name)
    WHERE connector = 'csv' AND source_name <> ''
      AND status IN ('validating', 'awaiting_approval', 'running', 'undoing');

CREATE UNIQUE INDEX import_run_one_live_per_source_ref ON import_run (source_ref)
    WHERE connector = 'csv' AND source_released_at IS NULL;

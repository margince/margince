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

-- One run per source name among the statuses that hold it. Two undos of one
-- name have no safe order, so the migration stops and names them.
DO $$
DECLARE
    clash text;
BEGIN
    SELECT string_agg(id::text, ', ' ORDER BY id) INTO clash
      FROM (SELECT id, count(*) OVER (PARTITION BY source_name) AS undos
              FROM import_run
             WHERE connector = 'csv' AND source_name <> '' AND status = 'undoing') named
     WHERE undos > 1;
    IF clash IS NOT NULL THEN
        RAISE EXCEPTION 'import runs % are undoing one source name at once', clash
            USING HINT = 'Press undo on each of these runs until all but one is undone, then run the migration again.';
    END IF;
END $$;

-- An undo keeps the name, else the newest commit, else the newest report. An
-- older commit stops, resumable from its checkpoint: migration.CauseStopped.
-- Every other report is superseded by the keeper: migration.CauseSuperseded.
WITH ranked AS (
    SELECT id,
           first_value(id) OVER named AS keeper,
           row_number() OVER named AS rank
      FROM import_run
     WHERE connector = 'csv' AND source_name <> ''
       AND status IN ('validating', 'awaiting_approval', 'running', 'undoing')
    WINDOW named AS (PARTITION BY source_name
                     ORDER BY status = 'undoing' DESC, status = 'running' DESC, updated_at DESC, id DESC)
)
UPDATE import_run r
   SET status = CASE WHEN r.status = 'running' THEN 'failed' ELSE 'cancelled' END,
       superseded_by = CASE WHEN r.status = 'running' THEN NULL ELSE ranked.keeper END,
       source_released_at = CASE WHEN r.status = 'running' THEN NULL ELSE now() END,
       error = CASE WHEN r.status = 'running'
                    THEN 'the import stopped on an error; approve it again to continue from its checkpoint'
                    ELSE 'a newer test run of this source replaced this one' END
  FROM ranked
 WHERE r.id = ranked.id AND ranked.rank > 1;

-- One unreleased run per upload. The run that still needs the file keeps it: a
-- commit in flight, else the newest resumable commit, else the newest run. A
-- released run that wrote nothing is superseded; a stopped commit that lost the
-- file says so (migration.CauseSourceTaken), and every other keeps its status.
WITH ranked AS (
    SELECT id,
           first_value(id) OVER upload AS keeper,
           row_number() OVER upload AS rank
      FROM import_run
     WHERE connector = 'csv' AND source_released_at IS NULL
    WINDOW upload AS (PARTITION BY source_ref
                      ORDER BY status = 'running' DESC, status = 'failed' DESC, created_at DESC, id DESC)
)
UPDATE import_run r
   SET source_released_at = now(),
       status = CASE WHEN r.status IN ('pending', 'validating', 'awaiting_approval')
                     THEN 'cancelled' ELSE r.status END,
       superseded_by = CASE WHEN r.status IN ('pending', 'validating', 'awaiting_approval')
                            THEN ranked.keeper ELSE r.superseded_by END,
       error = CASE WHEN r.status IN ('pending', 'validating', 'awaiting_approval')
                    THEN 'a newer test run of this source replaced this one'
                    WHEN r.status = 'failed'
                    THEN 'another run of this upload holds its file, so this one cannot resume; upload the file again to go on'
                    ELSE r.error END
  FROM ranked
 WHERE r.id = ranked.id AND ranked.rank > 1;

-- A key released here is declared provisional the way releasing a source does,
-- so the reaper deletes the bytes once no unreleased run names them.
INSERT INTO stored_object_intent (storage_key, kind)
SELECT DISTINCT source_ref, 'import'
  FROM import_run
 WHERE connector = 'csv' AND source_released_at IS NOT NULL AND source_ref <> ''
ON CONFLICT (storage_key) DO NOTHING;

-- Built plainly in the transaction that deduplicated, so no writer can add a
-- duplicate between the dedupe and the index. The table holds one row per import.
CREATE UNIQUE INDEX import_run_one_active_per_source_name ON import_run (source_name)
    WHERE connector = 'csv' AND source_name <> ''
      AND status IN ('validating', 'awaiting_approval', 'running', 'undoing');

CREATE UNIQUE INDEX import_run_one_live_per_source_ref ON import_run (source_ref)
    WHERE connector = 'csv' AND source_released_at IS NULL;

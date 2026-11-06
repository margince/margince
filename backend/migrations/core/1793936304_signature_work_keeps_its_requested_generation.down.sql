-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

SET LOCAL lock_timeout = '5s';

DROP TRIGGER reopen_deleted_signature_source ON activity;
DROP FUNCTION reopen_deleted_signature_source();
DROP INDEX idx_signature_work_due;
DROP INDEX idx_signature_live_work_due;
DROP INDEX idx_signature_work_deferred;
DO $$BEGIN
 IF EXISTS (SELECT 1 FROM contact_signature_enrich_state WHERE activity_id IS NULL OR last_activity_at IS NULL OR attempted_at IS NULL OR requested_generation>processed_generation) THEN
  RAISE EXCEPTION 'Finish pending signature reads before removing their progress columns';
 END IF;
END$$;
ALTER TABLE contact_signature_enrich_state DROP CONSTRAINT contact_signature_enrich_state_activity_id_fkey;
ALTER TABLE contact_signature_enrich_state ADD CONSTRAINT contact_signature_enrich_state_activity_id_fkey
    FOREIGN KEY (activity_id) REFERENCES activity(id) ON DELETE CASCADE NOT VALID;
ALTER TABLE contact_signature_enrich_state
    DROP CONSTRAINT signature_work_generations,
    DROP COLUMN live_generation, DROP COLUMN requested_generation, DROP COLUMN processed_generation, DROP COLUMN due_at, DROP COLUMN deferred_run_id,
    ALTER COLUMN attempted_at SET NOT NULL, ALTER COLUMN activity_id SET NOT NULL, ALTER COLUMN last_activity_at SET NOT NULL;

-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

SET LOCAL lock_timeout = '5s';

ALTER TABLE contact_signature_enrich_state
    ALTER COLUMN activity_id DROP NOT NULL,
    ALTER COLUMN last_activity_at DROP NOT NULL,
    ALTER COLUMN attempted_at DROP NOT NULL,
    ADD COLUMN requested_generation bigint NOT NULL DEFAULT 0,
    ADD COLUMN processed_generation bigint NOT NULL DEFAULT 0,
    ADD COLUMN live_generation bigint NOT NULL DEFAULT 0,
    ADD COLUMN due_at timestamptz NOT NULL DEFAULT now(),
    ADD COLUMN deferred_run_id uuid,
    ADD CONSTRAINT signature_work_generations CHECK (processed_generation >= 0 AND requested_generation >= processed_generation AND live_generation >= 0 AND live_generation <= requested_generation) NOT VALID,
    ADD CONSTRAINT contact_signature_enrich_state_deferred_run_id_fkey FOREIGN KEY (deferred_run_id) REFERENCES capture_backfill(id) ON DELETE SET NULL NOT VALID;
ALTER TABLE contact_signature_enrich_state DROP CONSTRAINT contact_signature_enrich_state_activity_id_fkey;
ALTER TABLE contact_signature_enrich_state ADD CONSTRAINT contact_signature_enrich_state_activity_id_fkey
    FOREIGN KEY (activity_id) REFERENCES activity(id) ON DELETE SET NULL NOT VALID;
CREATE INDEX idx_signature_work_due ON contact_signature_enrich_state (due_at, contact_id)
    WHERE requested_generation > processed_generation AND live_generation <= processed_generation AND deferred_run_id IS NULL;
CREATE INDEX idx_signature_live_work_due ON contact_signature_enrich_state (due_at, contact_id)
    WHERE requested_generation > processed_generation AND live_generation > processed_generation;
CREATE INDEX idx_signature_work_deferred ON contact_signature_enrich_state (deferred_run_id)
    WHERE deferred_run_id IS NOT NULL;

CREATE FUNCTION reopen_deleted_signature_source() RETURNS trigger LANGUAGE plpgsql AS $$BEGIN
    WITH affected AS (
        SELECT contact_id FROM contact_signature_enrich_state WHERE activity_id=OLD.id
        UNION SELECT contact_id FROM activity_link WHERE activity_id=OLD.id AND contact_id IS NOT NULL AND OLD.kind='email' AND OLD.direction='inbound'
        UNION SELECT contact_id FROM activity_participant WHERE activity_id=OLD.id AND contact_id IS NOT NULL AND OLD.kind='email' AND OLD.direction='inbound'
    ), subjects AS (
        SELECT c.id FROM contact c JOIN affected a ON a.contact_id=c.id ORDER BY c.id FOR UPDATE OF c
    )
    INSERT INTO contact_signature_enrich_state AS st (contact_id,requested_generation,live_generation,attempted_at)
    SELECT id,1,1,NULL FROM subjects ORDER BY id
    ON CONFLICT (contact_id) DO UPDATE
       SET activity_id=NULL,last_activity_at=NULL,history_depth=NULL,
           requested_generation=st.requested_generation+1,live_generation=st.requested_generation+1,due_at=now(),deferred_run_id=NULL;
    RETURN OLD;
END;
$$;
CREATE TRIGGER reopen_deleted_signature_source BEFORE DELETE ON activity
    FOR EACH ROW EXECUTE FUNCTION reopen_deleted_signature_source();

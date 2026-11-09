-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

SET LOCAL lock_timeout = '3s';

ALTER TABLE activity
    DROP CONSTRAINT activity_legal_entity_id_fkey,
    ADD CONSTRAINT activity_legal_entity_id_fkey FOREIGN KEY (legal_entity_id)
        REFERENCES legal_entity(id) ON DELETE RESTRICT NOT VALID;

ALTER TABLE capture_connection
    DROP CONSTRAINT capture_connection_legal_entity_id_fkey,
    ADD CONSTRAINT capture_connection_legal_entity_id_fkey FOREIGN KEY (legal_entity_id)
        REFERENCES legal_entity(id) ON DELETE RESTRICT NOT VALID;

ALTER TABLE capture_import
    DROP CONSTRAINT capture_import_legal_entity_id_fkey,
    ADD CONSTRAINT capture_import_legal_entity_id_fkey FOREIGN KEY (legal_entity_id)
        REFERENCES legal_entity(id) ON DELETE RESTRICT NOT VALID;

ALTER TABLE contact_acquisition_evidence
    DROP CONSTRAINT contact_acquisition_evidence_legal_entity_id_fkey,
    ADD CONSTRAINT contact_acquisition_evidence_legal_entity_id_fkey FOREIGN KEY (legal_entity_id)
        REFERENCES legal_entity(id) ON DELETE RESTRICT NOT VALID;


-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

-- Validation follows the transaction that added the columns, so readers do not wait on its table locks.
SET LOCAL lock_timeout = '3s';

ALTER TABLE activity VALIDATE CONSTRAINT activity_legal_entity_id_fkey;
ALTER TABLE capture_connection VALIDATE CONSTRAINT capture_connection_legal_entity_id_fkey;
ALTER TABLE capture_import VALIDATE CONSTRAINT capture_import_legal_entity_id_fkey;
ALTER TABLE contact_acquisition_evidence VALIDATE CONSTRAINT contact_acquisition_evidence_legal_entity_id_fkey;

-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

SET LOCAL lock_timeout = '3s';

ALTER TABLE consent_qualifying_event DROP COLUMN recorded_for_entities;
ALTER TABLE contact_acquisition_evidence DROP COLUMN legal_entity_id;
ALTER TABLE capture_import DROP COLUMN legal_entity_id;
ALTER TABLE capture_connection DROP COLUMN legal_entity_id;
ALTER TABLE activity DROP COLUMN legal_entity_id;

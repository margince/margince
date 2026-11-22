-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

SET LOCAL lock_timeout = '5s';

ALTER TABLE approval
    DROP CONSTRAINT IF EXISTS approval_target_entity_type_check,
    DROP CONSTRAINT IF EXISTS approval_co_target_entity_type_check;
ALTER TABLE contact_acquisition_evidence
    DROP CONSTRAINT IF EXISTS contact_acquisition_evidence_source_entity_type_check;

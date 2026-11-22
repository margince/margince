-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

-- The constraints added NOT VALID by the file before this one are validated here,
-- in their own transaction, so the scan runs under SHARE UPDATE EXCLUSIVE.
SET LOCAL lock_timeout = '5s';

ALTER TABLE approval VALIDATE CONSTRAINT approval_target_entity_type_check;
ALTER TABLE approval VALIDATE CONSTRAINT approval_co_target_entity_type_check;
ALTER TABLE contact_acquisition_evidence VALIDATE CONSTRAINT contact_acquisition_evidence_source_entity_type_check;

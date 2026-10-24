-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

-- The two checks and the key added beside this file are validated here, under
-- SHARE UPDATE EXCLUSIVE, which readers and writers pass.
SET LOCAL lock_timeout = '3s';

ALTER TABLE legal_entity VALIDATE CONSTRAINT legal_entity_default_has_no_offering;
ALTER TABLE legal_entity VALIDATE CONSTRAINT legal_entity_offering_bounded;
ALTER TABLE company VALIDATE CONSTRAINT company_assess_against_entity_id_fkey;

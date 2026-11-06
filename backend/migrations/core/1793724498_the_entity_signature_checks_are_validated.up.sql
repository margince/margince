-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

-- The two checks added beside this file are validated here, under SHARE UPDATE
-- EXCLUSIVE, which readers and writers pass.
SET LOCAL lock_timeout = '3s';

ALTER TABLE legal_entity VALIDATE CONSTRAINT legal_entity_default_signs_with_the_installation;
ALTER TABLE legal_entity VALIDATE CONSTRAINT legal_entity_signature_template_bounded;

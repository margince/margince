-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

-- Every sender signs with the installation's template again.
SET LOCAL lock_timeout = '3s';

ALTER TABLE legal_entity
    DROP CONSTRAINT IF EXISTS legal_entity_signature_template_bounded,
    DROP CONSTRAINT IF EXISTS legal_entity_default_signs_with_the_installation,
    DROP COLUMN IF EXISTS signature_template;

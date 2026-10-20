-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

SET LOCAL lock_timeout = '3s';

ALTER TABLE custom_field DROP CONSTRAINT custom_field_reportable_type_check;
ALTER TABLE custom_field ADD CONSTRAINT custom_field_reportable_type_check
    CHECK (NOT reportable OR (object = 'deal' AND type IN ('picklist', 'boolean'))) NOT VALID;

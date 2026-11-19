-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

-- custom_field holds one row per field an admin defined, so the check validates at once.
SET LOCAL lock_timeout = '2s';
ALTER TABLE custom_field DROP CONSTRAINT IF EXISTS custom_field_type_check;
ALTER TABLE custom_field ADD CONSTRAINT custom_field_type_check
 CHECK (type IN ('text', 'number', 'date', 'currency', 'picklist', 'multiselect', 'boolean', 'record', 'url'));

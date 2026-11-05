-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

-- A company's single-choice or yes/no field can now be opted into reporting as
-- well as a deal's. Deal reports reach it through the deal's company. NOT VALID
-- skips the row scan; the next migration validates it.
SET LOCAL lock_timeout = '3s';

ALTER TABLE custom_field DROP CONSTRAINT custom_field_reportable_type_check;
ALTER TABLE custom_field ADD CONSTRAINT custom_field_reportable_type_check
    CHECK (NOT reportable OR (object IN ('deal', 'company') AND type IN ('picklist', 'boolean'))) NOT VALID;

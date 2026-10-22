-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

-- An administrator opts a custom field into reporting. Off by default: a field
-- has no read mask of its own, so a report may group by it only once someone
-- with authority over the catalog has said its values are fit to aggregate.
-- Only a deal's single-choice or yes/no field can be: reports group deals
-- alone by custom fields, and one value per record keeps a total from counting
-- a record twice. NOT VALID skips the row scan; every row is false here.
SET LOCAL lock_timeout = '3s';

ALTER TABLE custom_field ADD COLUMN reportable boolean NOT NULL DEFAULT false;
ALTER TABLE custom_field ADD CONSTRAINT custom_field_reportable_type_check
    CHECK (NOT reportable OR (object = 'deal' AND type IN ('picklist', 'boolean'))) NOT VALID;

-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

-- An administrator opts a custom field into reporting. Off by default: a field
-- has no read mask of its own, so a report may group by it only once someone
-- with authority over the catalog has said its values are fit to aggregate.
-- Only a single-choice or yes/no field can be: one value per record is what
-- keeps a grouped total from counting a record twice.
SET LOCAL lock_timeout = '3s';

ALTER TABLE custom_field ADD COLUMN reportable boolean NOT NULL DEFAULT false;
ALTER TABLE custom_field ADD CONSTRAINT custom_field_reportable_type_check
    CHECK (NOT reportable OR type IN ('picklist', 'boolean'));

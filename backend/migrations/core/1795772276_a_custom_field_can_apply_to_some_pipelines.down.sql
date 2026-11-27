-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

SET LOCAL lock_timeout = '3s';

ALTER TABLE custom_field DROP CONSTRAINT custom_field_pipeline_ids_check;
ALTER TABLE custom_field DROP COLUMN pipeline_ids;

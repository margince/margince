-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

-- Validates the reporting CHECK the previous migration added NOT VALID, in a
-- transaction of its own so the scan runs under SHARE UPDATE EXCLUSIVE. Every
-- stored row passes: the column was false on every row when it was added.
SET LOCAL lock_timeout = '3s';

ALTER TABLE custom_field VALIDATE CONSTRAINT custom_field_reportable_type_check;

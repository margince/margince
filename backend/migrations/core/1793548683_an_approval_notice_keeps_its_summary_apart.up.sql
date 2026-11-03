-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

-- An approval notice's subject taken apart into its sentence and field values,
-- so the bell shows the values as a list. A notice written before this is null.
SET LOCAL lock_timeout = '3s';

ALTER TABLE notice ADD COLUMN subject_parts jsonb;

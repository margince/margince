-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

-- The summary's sentence and its field values, kept apart so a reader can show
-- them apart. summary stays the joined line; a row staged before this is null.
SET LOCAL lock_timeout = '3s';

ALTER TABLE approval ADD COLUMN summary_parts jsonb;

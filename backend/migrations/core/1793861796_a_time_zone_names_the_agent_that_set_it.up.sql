-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

-- NULL means a user set the zone, or a browser guessed it, as all did before this column.
SET LOCAL lock_timeout = '3s';

ALTER TABLE app_user ADD COLUMN timezone_set_by text;

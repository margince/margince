-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

-- The reverse. The stored zone itself stays.
SET LOCAL lock_timeout = '3s';

ALTER TABLE app_user DROP COLUMN IF EXISTS timezone_chosen;

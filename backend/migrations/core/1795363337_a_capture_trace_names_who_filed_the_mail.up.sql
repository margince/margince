-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

-- Who filed a mail on its deal, for Settings > Capture. A nullable column with
-- no default is a catalog change only, so the lock is instant.
SET LOCAL lock_timeout = '3s';

ALTER TABLE capture_trace ADD COLUMN IF NOT EXISTS filing jsonb;

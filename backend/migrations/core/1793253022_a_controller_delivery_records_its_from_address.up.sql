-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

SET LOCAL lock_timeout = '2s';

-- The address a controller delivery left from, or was staged with while it
-- waits. NULL when the relay names none.
ALTER TABLE comms_outbound ADD COLUMN IF NOT EXISTS from_address text;

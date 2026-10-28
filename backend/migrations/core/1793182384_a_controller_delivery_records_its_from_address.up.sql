-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

SET LOCAL lock_timeout = '2s';

-- The address a controller delivery was staged to be sent from. NULL on one
-- staged before it was recorded.
ALTER TABLE comms_outbound ADD COLUMN IF NOT EXISTS from_address text;

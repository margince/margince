-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

-- Back to working out the decider on read. Requests this migration expired stay
-- expired: reopening them would put stale proposals back in front of people.
SET LOCAL lock_timeout = '5s';

ALTER TABLE approval DROP CONSTRAINT IF EXISTS approval_pending_names_decider;
ALTER TABLE approval DROP CONSTRAINT IF EXISTS approval_requested_decider_source_check;
ALTER TABLE approval DROP CONSTRAINT IF EXISTS approval_requested_decider_fkey;
ALTER TABLE approval DROP COLUMN IF EXISTS requested_decider_source;
ALTER TABLE approval DROP COLUMN IF EXISTS requested_decider_id;

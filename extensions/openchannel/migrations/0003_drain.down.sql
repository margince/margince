-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion
--
-- Reversing this destroys the record of every message this connector sent
-- outward, and the link between a queued request and the timeline entry it
-- became. Neither is a copy of anything held elsewhere.
--
-- The withdrawn rows are moved back to `ingested` BEFORE the state constraint
-- narrows again: they name a timeline entry that is archived either way, and the
-- alternative to naming the closest surviving state is a revert that fails on
-- the constraint it is restoring.

DROP TABLE IF EXISTS ext.ext_openchannel_outbound;

DROP INDEX IF EXISTS ext.ext_openchannel_inbound_activity_idx;
DROP INDEX IF EXISTS ext.ext_openchannel_inbound_state_idx;

UPDATE ext.ext_openchannel_inbound SET state = 'ingested' WHERE state = 'withdrawn';

ALTER TABLE ext.ext_openchannel_inbound DROP CONSTRAINT ext_openchannel_inbound_state_check;
ALTER TABLE ext.ext_openchannel_inbound ADD CONSTRAINT ext_openchannel_inbound_state_check
    CHECK (state IN ('pending', 'ingested', 'failed'));

ALTER TABLE ext.ext_openchannel_inbound DROP COLUMN IF EXISTS activity_id;

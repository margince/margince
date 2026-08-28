-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion
--
-- Dropping the table takes its index, its grants and its foreign key with it,
-- which is why nothing else is spelled here. It also destroys every queued
-- request that had not yet been acted on: those are evidence of arrivals, not a
-- copy of anything a sender still holds, so a reader reversing this migration
-- is deleting the only record of them.

DROP TABLE IF EXISTS ext.ext_openchannel_inbound;

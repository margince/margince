-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion
--
-- Reversing this only returns the retention sweep to a plan with no index
-- behind its predicate; it destroys no row and no evidence.

DROP INDEX IF EXISTS ext.ext_openchannel_inbound_decided_updated_idx;

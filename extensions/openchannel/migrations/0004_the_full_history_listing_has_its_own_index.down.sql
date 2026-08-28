-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion
--
-- Reversing this only returns the full-history listing to a plan that sorts
-- what it reads; it destroys no row and no evidence.

DROP INDEX IF EXISTS ext.ext_openchannel_inbound_endpoint_received_idx;

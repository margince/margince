-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion
--
-- retention.go's sweepDecided keys its window on updated_at — WHEN A REQUEST
-- WAS DECIDED — not received_at, and filters to rows whose state is no longer
-- `pending`. The 0003 index (state, received_at) does not serve that: it is
-- ordered by the wrong column for a range on updated_at, so every drain tick
-- swept the retained rows with no index behind the predicate at all, scanning
-- the whole table to find the handful past the retention window.
--
-- Partial, and on the exact predicate the sweep uses (state <> 'pending'):
-- pending rows are never a sweep target and are the majority of a busy queue,
-- so indexing them would be indexing rows this query never touches.

CREATE INDEX ext_openchannel_inbound_decided_updated_idx
    ON ext.ext_openchannel_inbound (updated_at)
    WHERE state <> 'pending';

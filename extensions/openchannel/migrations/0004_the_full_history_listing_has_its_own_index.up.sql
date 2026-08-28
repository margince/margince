-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion
--
-- listInbound reads one endpoint's whole history — every state, newest first
-- — which is a different question from the one the 0002 index answers. That
-- one is keyed (endpoint_id, state, received_at): correct for "how many are
-- still pending" and "which to take next", both of which filter on state
-- before they need an order, but useless for an ORDER BY received_at that
-- spans every state, because a scan constrained to one endpoint_id and no
-- state still has to merge four state-grouped runs of received_at instead of
-- reading one. Without an index whose second column is received_at, that
-- listing degrades into a full scan of the endpoint's rows plus a sort as the
-- retained history grows — the exact shape retention.go's own window is sized
-- against, not a bound on how many rows a busy endpoint may accumulate first.

CREATE INDEX ext_openchannel_inbound_endpoint_received_idx
    ON ext.ext_openchannel_inbound (endpoint_id, received_at DESC);

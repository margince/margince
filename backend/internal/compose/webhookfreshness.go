// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import "time"

// withinSkew is the freshness comparison shared by the extension inbound
// edge's admission (extinbound.go), which parses epoch SECONDS, and the
// HubSpot receiver's freshTimestamp (overlaywebhook.go), which parses epoch
// MILLISECONDS.
//
// Two things are easy to get wrong here and both are one-directional bugs, so
// they are held in one place rather than in each caller: the distance is
// ABSOLUTE, because a sender with a fast clock would otherwise mint requests
// that stay valid past the window; and the bound is inclusive, so a skew of
// exactly Skew is fresh rather than falling in a gap between the two callers'
// spellings.
// Compared as INSTANTS rather than by subtracting them. time.Sub saturates at
// ±(1<<63-1) nanoseconds — about 292 years — and the saturated negative value
// negates to itself, still negative, so an absolute-value spelling admits every
// timestamp far enough in the future to overflow. That is precisely the
// fast-clock sender this bound exists to refuse, so the arithmetic has to be
// the kind that cannot wrap. now±skew is always in range: skew is bounded by
// MaxInboundSkew and now is the wall clock.
func withinSkew(now, at time.Time, skew time.Duration) bool {
	return !at.Before(now.Add(-skew)) && !at.After(now.Add(skew))
}

// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package gcal

import "time"

// captureForwards is how far ahead of now an event is captured.
//
// The list expands every recurring series into its occurrences
// (singleEvents=true), and Google expands a series with no end for decades: one
// monthly meeting landed 393 rows on the timeline, the last in 2056. Google
// refuses timeMax beside a syncToken, so the bound is applied here, to every
// pull. A year matches the Microsoft connector's forward window
// (graphcal.viewForwards), so one meeting reads the same on either calendar.
const captureForwards = 365 * 24 * time.Hour

// relistAfter is how long a full list is trusted before it is taken again.
//
// An occurrence that was past the horizon when the list was taken does not
// change as time passes, so no incremental pull will ever mention it. Re-listing
// monthly is what brings it in once it comes within a year — the same cadence
// the Microsoft connector re-anchors its window on (graphcal.reanchorAfter).
const relistAfter = 30 * 24 * time.Hour

// listIsStale reports whether the full list the cursor descends from is old
// enough to take again. A zero date is a cursor from before this was recorded:
// re-list once, and it starts carrying one.
func (c *Connector) listIsStale(listedAt time.Time) bool {
	return listedAt.IsZero() || c.now().Sub(listedAt) >= relistAfter
}

// beyondHorizon reports that one raw event starts after the capture edge and is
// not captured on this pull.
//
// A cancellation or a declined invitation is never held back: it may close a
// meeting captured before this bound existed, and it writes no new row. An
// event that will not decode is left to CaptureOne, which already skips it.
func beyondHorizon(raw []byte, owner string, edge time.Time) bool {
	ev, err := decodeEvent(raw, owner)
	if err != nil || ev.Cancelled || ev.OwnerDeclined {
		return false
	}
	return ev.StartsAt.After(edge)
}

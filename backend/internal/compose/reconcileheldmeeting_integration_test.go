// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import "testing"

// A past meeting grounds a follow-up only once someone recorded it held. One
// with no status, or still booked, is awaiting its outcome and is no
// conversation to follow up on.
func TestFollowUpReconcileGroundsOnlyOnAHeldMeeting(t *testing.T) {
	e := setupReconcile(t)
	for _, status := range []string{"", "booked"} {
		unreported := e.SeedDeal(t, "Met, nobody reported it ("+status+")", e.pipeline, e.open, &e.Rep1)
		e.seedMeeting(t, unreported, "Workshop", 3, 1800, status)
		if err := e.reconcile(); err != nil {
			t.Fatal(err)
		}
		if got := e.followUps(t, unreported); got != 0 {
			t.Errorf("a past meeting with status %q staged %d follow-ups, want none", status, got)
		}
	}

	held := e.SeedDeal(t, "Met and reported", e.pipeline, e.open, &e.Rep1)
	e.seedMeeting(t, held, "Workshop", 3, 1800, "held")
	if err := e.reconcile(); err != nil {
		t.Fatal(err)
	}
	if _, kind, _ := e.followUpEvidence(t, held); kind != "meeting" {
		t.Errorf("the follow-up cites a %q, want the held meeting", kind)
	}
}

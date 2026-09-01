// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package introductions

import "testing"

// The tab is told about every status that holds a route, and about refusals.
//
// The way this fails without a check is the silent one: a status that holds a
// route but is missing from the list is simply not selected, the tab shows the
// route as free, and the rep is refused by the index after writing the ask —
// which is the exact defect this read exists to remove, reappearing for one
// state instead of all of them.
func TestRouteStatesReportsEveryOpenStatus(t *testing.T) {
	reported := map[string]bool{}
	for _, s := range statesWorthReporting() {
		reported[s] = true
	}

	for _, s := range everyStatus() {
		if Open(s) && !reported[string(s)] {
			t.Errorf("%q holds a route and is not reported, so the tab would "+
				"offer a route the duplicate guard refuses", s)
		}
		if !Open(s) && s != StatusDeclined && reported[string(s)] {
			t.Errorf("%q is settled and is reported, so the tab would block a "+
				"route the server would accept", s)
		}
	}

	// The refusal is carried on purpose: it is not a bar, and the tab says the
	// colleague answered no rather than re-recommending the same door.
	if !reported[string(StatusDeclined)] {
		t.Error("a refusal is not reported, so a route that was refused reads as untried")
	}
}

// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"os"
	"regexp"
	"testing"

	"github.com/margince/margince/backend/internal/compose/network"
	"github.com/margince/margince/backend/internal/modules/introductions"
)

// Every state the introductions module can report has a word on the network
// side.
//
// The way this fails without a test is silent and one-directional: a state
// this mapping drops is absent from the map, an absent key reads as the zero
// RouteState, and the zero state leaves the route AVAILABLE. So a new kind of
// "this route is spoken for" would show up as "this route is free" — the exact
// defect the seam exists to fix, reintroduced by addition rather than by edit.
// The states come out of the module's own source rather than a list here,
// because a list here is a census that can fail short: a state added there and
// forgotten here would leave this test iterating the two it always knew and
// reporting PASS over the gap it exists to find.
func TestTheRouteSeamTranslatesEveryState(t *testing.T) {
	source, err := os.ReadFile(
		"../modules/introductions/routestate.go")
	if err != nil {
		t.Fatalf("reading the states: %v", err)
	}
	declared := regexp.MustCompile(
		`Route[A-Za-z]+ RouteState = "([a-z_]+)"`).FindAllSubmatch(source, -1)
	if len(declared) == 0 {
		t.Fatal("no route states found — this gate has stopped reading its subject")
	}

	for _, m := range declared {
		state := introductions.RouteState(m[1])
		translated, ok := networkRouteState(state)
		if !ok {
			t.Errorf("%q has no word on the network side, so a route in that "+
				"state would be offered as free", state)
			continue
		}
		if translated != network.RouteOpen && translated != network.RouteRefused {
			t.Errorf("%q translated to %q, which is neither state the tab renders",
				state, translated)
		}
	}
}

// A state with no translation blocks rather than opening the door.
//
// The introductions module reported it precisely BECAUSE the route is spoken
// for. Defaulting an unknown state to "free" would turn every future addition
// into a silent regression; defaulting to "taken" costs at worst a route the
// rep could have used, and the server tells them so when they ask.
func TestAnUntranslatableStateIsNotTreatedAsFree(t *testing.T) {
	if _, ok := networkRouteState(introductions.RouteState("something_new")); ok {
		t.Fatal("an unknown state claimed a translation")
	}
}

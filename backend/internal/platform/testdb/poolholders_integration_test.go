// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package testdb

// Who asks the pool whether it is quiet, and when.
//
// The observation itself is pinned next door (quiesce_integration_test.go). What
// is pinned here is the bookkeeping on top of it, because that is the half that
// can fail SILENTLY: a "last holder out" that never reports last means the
// assertion never runs, every suite goes green, and there is no failing
// assertion to notice. The gate this replaced failed the other way — loudly and
// wrongly — which is how it was found at all.

import (
	"slices"
	"testing"
)

// Each case takes the package-level bookkeeping, so they may not run in
// parallel with each other — and must leave it empty, or the next test in this
// process reports a group that was never running.
func withNoHolders(t *testing.T) {
	t.Helper()
	holdersMu.Lock()
	before, beforeCount := sharing, finished
	sharing, finished = nil, 0
	holdersMu.Unlock()
	t.Cleanup(func() {
		holdersMu.Lock()
		sharing, finished = before, beforeCount
		holdersMu.Unlock()
	})
}

// A test that finishes while a sibling is still running must NOT be the one to
// ask: the connection the sibling holds is its work, and asking now reports it
// as this test's leak — confidently, naming whichever test happened to end
// first, and only on a runner loaded enough for the overlap to matter.
func TestAHolderThatLeavesSiblingsRunningDoesNotAsk(t *testing.T) {
	withNoHolders(t)

	holdPool("first")
	holdPool("second")

	if _, last := releasePool(); last {
		t.Error("the first of two holders out asked the pool whether it was quiet — its sibling " +
			"is still running, and whatever that sibling holds would read as this test's leak")
	}
	group, last := releasePool()
	if !last {
		t.Fatal("the second of two holders out did not ask — nobody asks, and the gate is off")
	}
	if !slices.Equal(group, []string{"first", "second"}) {
		t.Errorf("the group was %v, want both holders — a failure names the tests that shared "+
			"the pool, because pgx records no owner for a connection", group)
	}
}

// A serial package is the common case and must be unchanged: one holder at a
// time, each asking on its way out, each reporting only itself.
func TestASerialHolderAsksOnItsOwnWayOut(t *testing.T) {
	withNoHolders(t)

	for _, name := range []string{"one", "two"} {
		holdPool(name)
		group, last := releasePool()
		if !last {
			t.Fatalf("%s ran alone and did not ask — a serial suite would never check its pool", name)
		}
		if !slices.Equal(group, []string{name}) {
			t.Errorf("%s reported the group %v, want only itself: a group that never emptied would "+
				"name every test the package had run", name, group)
		}
	}
}

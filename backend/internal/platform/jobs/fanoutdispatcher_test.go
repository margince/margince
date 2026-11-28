// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package jobs

import (
	"slices"
	"testing"
)

// TestFanOutDispatcherKindsSelectsExactlyTheKindsThatEnqueueChildren —
// statsBySweep leaves a dispatcher's own row out, because the children it
// enqueues (a different kind) are the real signal, and counting the
// dispatcher's row too would double the pass. A kind that fans out to nothing
// is a pass of its own and must stay in the sweep read.
//
// Walks allSpecs() (core plus any composed/extension declarations), not the
// core-only specs map, so an extension's own pass is held to the same rule.
func TestFanOutDispatcherKindsSelectsExactlyTheKindsThatEnqueueChildren(t *testing.T) {
	t.Parallel()

	dispatchers := fanOutDispatcherKinds()
	var fanning, alone int
	for kind, spec := range allSpecs() {
		if spec.FanOutTo != "" {
			fanning++
			if !slices.Contains(dispatchers, kind) {
				t.Errorf("%s fans out to %s but is read as a pass of its own; its row would double-count what its children already report", kind, spec.FanOutTo)
			}
			continue
		}
		alone++
		if slices.Contains(dispatchers, kind) {
			t.Errorf("%s fans out to nothing but is left out of the sweep read", kind)
		}
	}
	// Both sides must be exercised by real declarations, or this gate passes
	// on nothing.
	if fanning == 0 {
		t.Fatal("no fan-out dispatcher declared; the excluded side of the partition is unexercised")
	}
	if alone == 0 {
		t.Fatal("no kind without a fan-out declared; the counted side of the partition is unexercised")
	}
}

// TestFanOutDispatcherKindsOfIgnoresAKindWithoutAChild — against a table this
// test builds.
func TestFanOutDispatcherKindsOfIgnoresAKindWithoutAChild(t *testing.T) {
	t.Parallel()

	kinds := fanOutDispatcherKindsOf(map[string]Spec{
		"a_pass":       {Kind: "a_pass", Fleet: true},
		"a_dispatcher": {Kind: "a_dispatcher", Fleet: true, FanOutTo: "a_dispatcher_child"},
		"a_worker":     {Kind: "a_worker"},
	})
	if !slices.Equal(kinds, []string{"a_dispatcher"}) {
		t.Errorf("selected %v, want only the kind that fans out", kinds)
	}
}

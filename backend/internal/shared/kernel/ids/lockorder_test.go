// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ids

import (
	"slices"
	"testing"
)

// The property every caller depends on: two callers handed the same rows in
// different orders arrive at the same order. That, and nothing about which
// order it is, is what stops them deadlocking.
func TestTwoCallersReachTheSameOrderFromDifferentOnes(t *testing.T) {
	t.Parallel()

	a, b, c := New[ActivityKind](), New[ActivityKind](), New[ActivityKind]()
	one := Ascending([]ID[ActivityKind]{a, b, c})
	other := Ascending([]ID[ActivityKind]{c, a, b})

	if !slices.Equal(one, other) {
		t.Errorf("two orderings of the same three rows sorted to %v and %v — callers that "+
			"disagree about lock order are what deadlock", one, other)
	}
	if len(one) != 3 {
		t.Errorf("three rows sorted to %d — an order that drops a row would leave it unlocked", len(one))
	}
}

// The caller's own slice is often the result set it also reports on, so
// reordering it underneath would change what a count or a first-element read
// means.
func TestSortingLeavesTheCallersSliceAlone(t *testing.T) {
	t.Parallel()

	a, b := New[ActivityKind](), New[ActivityKind]()
	given := []ID[ActivityKind]{b, a}
	first := given[0]

	Ascending(given)

	if given[0] != first {
		t.Error("Ascending reordered the caller's slice — a caller reporting on the set it " +
			"passed would find it rearranged underneath")
	}
}

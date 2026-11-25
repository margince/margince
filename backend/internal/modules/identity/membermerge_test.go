// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package identity

import "testing"

// A side owes a confirmation only when the source was used and the side's
// holder can still sign in. Every row of the table holds one answer.
func TestAMergeSideOwesAConfirmationOnlyWhileItsHolderCanSignIn(t *testing.T) {
	confirmed, pending := mergeToken{Confirmed: true}, mergeToken{}
	cases := []struct {
		name        string
		sourceUsed  bool
		deactivated bool
		token       *mergeToken
		want        string
	}{
		{"an unused source asks nobody", false, false, nil, confirmationNotNeeded},
		{"an unused source ignores a stray token", false, false, &confirmed, confirmationNotNeeded},
		{"a deactivated side needs none", true, true, nil, confirmationNotNeeded},
		{"a deactivated side ignores a token it still holds", true, true, &confirmed, confirmationNotNeeded},
		{"an active side nobody asked", true, false, nil, confirmationNotRequested},
		{"an active side asked and silent", true, false, &pending, confirmationRequested},
		{"an active side that confirmed", true, false, &confirmed, confirmationConfirmed},
	}
	for _, c := range cases {
		var token mergeToken
		if c.token != nil {
			token = *c.token
		}
		if got := mergeSideState(c.sourceUsed, c.deactivated, token, c.token != nil); got != c.want {
			t.Errorf("%s: state = %q, want %q", c.name, got, c.want)
		}
	}
}

// Both sides together: the merge waits while any side is open. Two active used
// accounts therefore always need both holders, and an administrator alone never
// merges them.
func TestAMergeWaitsWhileAnySideIsOpen(t *testing.T) {
	type side struct{ deactivated, confirmed bool }
	var all []side
	for _, deactivated := range []bool{false, true} {
		for _, confirmed := range []bool{false, true} {
			all = append(all, side{deactivated, confirmed})
		}
	}
	for _, source := range all {
		for _, survivor := range all {
			open := false
			for _, s := range []side{source, survivor} {
				token := mergeToken{Confirmed: s.confirmed}
				open = open || stillOpen(mergeSideState(true, s.deactivated, token, s.confirmed))
			}
			wantOpen := (!source.deactivated && !source.confirmed) || (!survivor.deactivated && !survivor.confirmed)
			if open != wantOpen {
				t.Errorf("source %+v, survivor %+v: open = %v, want %v", source, survivor, open, wantOpen)
			}
		}
	}
}

// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package dealstatus

// What a set-aside deal move is held against. "Not now" hides the move whose
// fingerprint the reader stored. Two moves a reader can tell apart must not
// share one, and a model rewriting the reason must not change it.

import (
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestOpeningWithTwoDifferentContactsIsSetAsideApart(t *testing.T) {
	alice := facts{deal: openDeal(), now: testNow, loc: time.UTC, seats: []Seat{
		{Role: "champion", Name: "Alice Brandt", ContactID: ids.NewV7()},
	}}
	bob := facts{deal: openDeal(), now: testNow, loc: time.UTC, seats: []Seat{
		{Role: "champion", Name: "Bob Kranz", ContactID: ids.NewV7()},
	}}
	a, b := decideMove(alice), decideMove(bob)
	if a.Action != ActionDraftEmail || b.Action != ActionDraftEmail {
		t.Fatalf("moves are %q and %q, want two first emails", a.Action, b.Action)
	}
	if a.Fingerprint == nil || b.Fingerprint == nil {
		t.Fatal("a first-outreach move carries no fingerprint to set it aside by")
	}
	if *a.Fingerprint == *b.Fingerprint {
		t.Error(`"open with Alice" and "open with Bob" share a fingerprint; setting one aside hides the other`)
	}
}

func TestARewrittenReasonKeepsTheMovesFingerprint(t *testing.T) {
	f := facts{deal: openDeal(), now: testNow, loc: time.UTC, seats: []Seat{
		{Role: "champion", Name: "Alice Brandt", ContactID: ids.NewV7()},
	}}
	mv := decideMove(f)
	written := writtenMove(mv, WrittenStatus{}, f)
	written.Reason = "A model's own words about the same move."
	if fingerprintOf(*written) != fingerprintOf(mv) {
		t.Error("rewording the reason changed the fingerprint, so a set-aside move returns on the next rewrite")
	}
}

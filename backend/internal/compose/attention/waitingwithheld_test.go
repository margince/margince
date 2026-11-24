// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package attention

import (
	"testing"
	"time"
)

// Mail no model may judge never gains a verdict. A customer answering on a
// conversation the seat owing the reply wrote on first stands in for one; nothing else does.
func TestWithheldMailRanksAsARequestOnlyWhenTheOwnerWroteFirst(t *testing.T) {
	for _, tc := range []struct {
		name                 string
		withheld, ownerWrote bool
		wantLevel            int
		wantConsequence      string
	}{
		{"withheld, the owner wrote first", true, true, levelWaiting, "buyer_waits"},
		{"withheld, the owner never wrote", true, false, levelRoutine, "none"},
		{"judgeable but unjudged, the owner wrote first", false, true, levelRoutine, "none"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			row := classifyWaiting(WaitingCustomer{
				Since: rankInstant.Add(-time.Hour), Engaged: true, ActionUnconfirmed: true,
				JudgeWithheld: tc.withheld, OwnerWroteOnThread: tc.ownerWrote,
			}, rankInstant)
			if row.item.Level != tc.wantLevel || string(row.item.Consequence) != tc.wantConsequence {
				t.Fatalf("ranked level %d with consequence %q, want %d and %q",
					row.item.Level, row.item.Consequence, tc.wantLevel, tc.wantConsequence)
			}
		})
	}
}

// A verdict outranks the fallback: mail judged to ask nothing stays routine
// even on a conversation the owner opened.
func TestAnInformsUsVerdictOutranksTheOwnerHavingWritten(t *testing.T) {
	row := classifyWaiting(WaitingCustomer{
		Since: rankInstant.Add(-time.Hour), Engaged: true, AsksNothing: true,
		JudgeWithheld: true, OwnerWroteOnThread: true,
	}, rankInstant)
	if row.item.Level != levelRoutine || string(row.item.Consequence) != "none" {
		t.Fatalf("an informs_us row ranked level %d with %q", row.item.Level, row.item.Consequence)
	}
}

// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package weekly

import "testing"

func TestDealRecoveryPrecedesCelebrationAndMeetingHygiene(t *testing.T) {
	recovery := dealRecoveryFocus(DealBlock{ForecastDown: 2, Open: 4, WithNextStep: 2})
	kind, label := focusFor(Counts{DealsWon: 3, MeetingsHeld: 2}, 0, recovery)
	if kind != FocusDealsAtRisk || label != "Forecast downgraded on 2 deals — review the recovery plan" {
		t.Fatalf("focus = %s %s", kind, label)
	}
}

func TestMissingDealHistoryDoesNotInventRecovery(t *testing.T) {
	if label := dealRecoveryFocus(DealBlock{}); label != "" {
		t.Fatalf("empty scorecard claimed %q", label)
	}
	for _, d := range []DealBlock{{Open: 3, CloseDateSound: 2}, {Open: 3, CloseDateSound: 3, WithNextStep: 2}, {Regressions: 1}} {
		if label := dealRecoveryFocus(d); label == "" {
			t.Fatal("recorded risk was omitted from agenda")
		}
	}
}

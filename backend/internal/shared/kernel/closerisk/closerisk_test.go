// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package closerisk

import "testing"

func TestACloseIsPassedBeforeTodayAndNearUpToTheHorizon(t *testing.T) {
	t.Parallel()
	for days, want := range map[int][2]bool{
		-1:                      {true, false},
		0:                       {false, true},
		RecoveryHorizonDays:     {false, true},
		RecoveryHorizonDays + 1: {false, false},
	} {
		if got := [2]bool{Passed(days), Near(days)}; got != want {
			t.Errorf("closes in %d days: passed, near = %v, want %v", days, got, want)
		}
	}
}

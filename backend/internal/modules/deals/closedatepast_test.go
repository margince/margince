// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package deals

import (
	"testing"
	"time"
)

func TestACloseDateIsPastOnlyBeforeTodaysDate(t *testing.T) {
	today := time.Date(2026, 7, 4, 0, 0, 0, 0, time.UTC)
	for _, tc := range []struct {
		date time.Time
		past bool
	}{
		{time.Date(2026, 7, 3, 23, 0, 0, 0, time.UTC), true},
		{time.Date(2026, 7, 4, 0, 0, 0, 0, time.UTC), false},
		// The day the date names counts, never the hour it was written at.
		{time.Date(2026, 7, 4, 23, 30, 0, 0, time.FixedZone("UTC+7", 7*3600)), false},
		{time.Date(2026, 7, 5, 0, 0, 0, 0, time.UTC), false},
	} {
		if got := CloseDateIsPast(tc.date, today); got != tc.past {
			t.Errorf("CloseDateIsPast(%v) = %v, want %v", tc.date, got, tc.past)
		}
	}
}

// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package licensecheck

import (
	"testing"
	"time"
)

// The trial covers fourteen whole days and not one instant more. There is no
// grace period, so the lock begins at EndsAt.
func TestTrialEndsFourteenDaysAfterItStartsWithNoGrace(t *testing.T) {
	start := time.Date(2026, 11, 2, 9, 30, 0, 0, time.UTC)
	trial := Trial{StartedAt: start}

	if want := time.Date(2026, 11, 16, 9, 30, 0, 0, time.UTC); !trial.EndsAt().Equal(want) {
		t.Fatalf("EndsAt = %v, want %v", trial.EndsAt(), want)
	}
	cases := []struct {
		name  string
		now   time.Time
		ended bool
	}{
		{"the day it starts", start, false},
		{"one second before the end", trial.EndsAt().Add(-time.Second), false},
		{"the instant it ends", trial.EndsAt(), true},
		{"a day later", trial.EndsAt().Add(24 * time.Hour), true},
	}
	for _, c := range cases {
		if got := trial.Ended(c.now); got != c.ended {
			t.Errorf("%s: Ended = %v, want %v", c.name, got, c.ended)
		}
	}
}

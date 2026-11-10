// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"testing"
	"time"
)

// now is the database's transaction time, the clock that stamped due.
func TestAPageDueNowStartsWhenItsInsertCommits(t *testing.T) {
	now := time.Date(2026, 11, 7, 9, 0, 0, 0, time.UTC)
	for name, due := range map[string]time.Time{
		"due at the transaction's time": now,
		"due in the past":               now.Add(-time.Minute),
	} {
		if got := successorStart(due, now); !got.IsZero() {
			t.Errorf("%s: ScheduledAt = %v, want zero so River makes the job available at commit", name, got)
		}
	}
}

func TestADelayWaitsForItsTime(t *testing.T) {
	now := time.Date(2026, 11, 7, 9, 0, 0, 0, time.UTC)
	for _, delay := range []time.Duration{time.Second, 10 * time.Second} {
		due := now.Add(delay)
		if got := successorStart(due, now); !got.Equal(due) {
			t.Errorf("a %s delay: ScheduledAt = %v, want %v", delay, got, due)
		}
	}
}

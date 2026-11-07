// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"testing"
	"time"
)

func TestAPageDueNowStartsWhenItsInsertCommits(t *testing.T) {
	now := time.Date(2026, 11, 7, 9, 0, 0, 0, time.UTC)
	for name, due := range map[string]time.Time{
		"due now":                         now,
		"due in the past":                 now.Add(-time.Minute),
		"database clock slightly ahead":   now.Add(500 * time.Millisecond),
		"database clock exactly at slack": now.Add(immediateSuccessorSlack),
	} {
		if got := successorStart(due, now); !got.IsZero() {
			t.Errorf("%s: ScheduledAt = %v, want zero so River makes the job available at commit", name, got)
		}
	}
}

func TestABackoffWaitsForItsTime(t *testing.T) {
	now := time.Date(2026, 11, 7, 9, 0, 0, 0, time.UTC)
	due := now.Add(10 * time.Second)
	if got := successorStart(due, now); !got.Equal(due) {
		t.Fatalf("ScheduledAt = %v, want the backoff's %v", got, due)
	}
}

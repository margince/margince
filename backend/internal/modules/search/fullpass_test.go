// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package search

import (
	"testing"
	"time"
)

// The first pass of a process and of each UTC day reads every row. The
// passes in between read only the recent rows.
func TestTheDriftSweepRunsOneFullPassPerDay(t *testing.T) {
	var d DailyFullPass
	morning := time.Date(2026, 11, 21, 0, 10, 0, 0, time.UTC)
	if got := d.Scope(morning); got != DriftFull {
		t.Fatalf("the first pass of a process has scope %v, want the full pass", got)
	}
	if got := d.Scope(morning.Add(15 * time.Minute)); got != DriftFull {
		t.Fatalf("a pass after a full pass that did not finish has scope %v, want the full pass again", got)
	}
	d.Finished(morning)
	if got := d.Scope(morning.Add(15 * time.Minute)); got != DriftRecent {
		t.Fatalf("a pass later on the day of a finished full pass has scope %v, want the recent pass", got)
	}
	if got := d.Scope(morning.Add(24 * time.Hour)); got != DriftFull {
		t.Fatalf("the first pass of the next day has scope %v, want the full pass", got)
	}
}

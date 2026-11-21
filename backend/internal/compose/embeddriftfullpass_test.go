// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/modules/search"
)

// The first pass of a process and of each UTC day reads every row. The
// passes in between read only the recent rows.
func TestTheDriftSweepRunsOneFullPassPerDay(t *testing.T) {
	var d dailyFullPass
	morning := time.Date(2026, 11, 21, 0, 10, 0, 0, time.UTC)
	if got := d.scope(morning); got != search.DriftFull {
		t.Fatalf("the first pass of a process has scope %v, want the full pass", got)
	}
	if got := d.scope(morning.Add(15 * time.Minute)); got != search.DriftFull {
		t.Fatalf("a pass after a full pass that did not finish has scope %v, want the full pass again", got)
	}
	d.finished(morning)
	if got := d.scope(morning.Add(15 * time.Minute)); got != search.DriftRecent {
		t.Fatalf("a pass later on the day of a finished full pass has scope %v, want the recent pass", got)
	}
	if got := d.scope(morning.Add(24 * time.Hour)); got != search.DriftFull {
		t.Fatalf("the first pass of the next day has scope %v, want the full pass", got)
	}
}

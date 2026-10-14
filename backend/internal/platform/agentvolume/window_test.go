// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package agentvolume

import (
	"testing"
	"time"
)

var tenAM = time.Date(2026, 10, 14, 10, 0, 0, 0, time.UTC)

func TestAPendingLengthTakesOverAtItsReset(t *testing.T) {
	plan := WindowPlan{Length: 24 * time.Hour}.Reschedule(tenAM, 12*time.Hour)

	midnight := time.Date(2026, 10, 15, 0, 0, 0, 0, time.UTC)
	if !plan.NextFrom.Equal(midnight) {
		t.Fatalf("the change starts at %v, want the next reset %v", plan.NextFrom, midnight)
	}
	if got := plan.At(midnight.Add(-time.Nanosecond)); got != 24*time.Hour {
		t.Errorf("before the reset the length is %v, want 24h", got)
	}
	if got := plan.At(midnight); got != 12*time.Hour {
		t.Errorf("at the reset the length is %v, want 12h", got)
	}
	if !plan.Pending(tenAM) || plan.Pending(midnight) {
		t.Error("the change should wait until the reset and no longer")
	}
}

func TestReschedulingToTheLengthInForceCancelsThePendingChange(t *testing.T) {
	plan := WindowPlan{Length: 24 * time.Hour}.Reschedule(tenAM, 12*time.Hour)

	plan = plan.Reschedule(tenAM.Add(time.Hour), 24*time.Hour)

	if plan.Pending(tenAM.Add(time.Hour)) || plan.Length != 24*time.Hour {
		t.Fatalf("asking for the length in force left %+v", plan)
	}
}

// Rescheduling after a pending change took over starts from the length that
// took over, not the one stored first.
func TestReschedulingSettlesAChangeThatAlreadyTookOver(t *testing.T) {
	plan := WindowPlan{Length: 24 * time.Hour}.Reschedule(tenAM, 12*time.Hour)
	later := time.Date(2026, 10, 15, 3, 0, 0, 0, time.UTC)

	plan = plan.Reschedule(later, 6*time.Hour)

	if plan.Length != 12*time.Hour {
		t.Fatalf("the length in force is %v, want the 12h that took over", plan.Length)
	}
	if want := time.Date(2026, 10, 15, 12, 0, 0, 0, time.UTC); !plan.NextFrom.Equal(want) {
		t.Errorf("the next change starts at %v, want the next 12h reset %v", plan.NextFrom, want)
	}
}

func TestAChangeSavedJustBeforeAResetWaitsForTheNextOne(t *testing.T) {
	justBefore := time.Date(2026, 10, 14, 23, 59, 30, 0, time.UTC)

	plan := WindowPlan{Length: 24 * time.Hour}.Reschedule(justBefore, 12*time.Hour)

	if want := time.Date(2026, 10, 16, 0, 0, 0, 0, time.UTC); !plan.NextFrom.Equal(want) {
		t.Fatalf("the change starts at %v, want %v: a process with a cached plan could miss a reset under a minute away", plan.NextFrom, want)
	}
}

func TestTheFirstPeriodUnderANewLengthIsNeverLonger(t *testing.T) {
	for _, tc := range []struct{ from, to time.Duration }{
		{6 * time.Hour, 24 * time.Hour},
		{24 * time.Hour, 7 * 24 * time.Hour},
		{time.Hour, 5 * time.Hour},
		{24 * time.Hour, time.Hour},
	} {
		plan := WindowPlan{Length: tc.from}.Reschedule(tenAM, tc.to)
		firstReset := ResetAfter(plan.NextFrom, tc.to)
		if period := firstReset.Sub(plan.NextFrom); period > tc.to || period <= 0 {
			t.Errorf("%v → %v: first period %v, want (0, %v]", tc.from, tc.to, period, tc.to)
		}
	}
}

// Two lengths in force at one moment name different buckets, so a counter of
// one length is never read as a counter of the other.
func TestNeighbouringLengthsNeverShareABucket(t *testing.T) {
	if bucketAt(tenAM, 167*time.Hour) == bucketAt(tenAM, 168*time.Hour) {
		t.Fatal("167h and 168h windows share a bucket number")
	}
}

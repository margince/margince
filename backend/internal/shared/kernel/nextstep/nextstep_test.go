// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package nextstep_test

import (
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/nextstep"
)

func TestAnOpenDealWithNothingBookedAndNothingFiledIsMissingItsNextStep(t *testing.T) {
	if !nextstep.Missing(nextstep.Facts{DealOpen: true}) {
		t.Fatal("a live deal with nothing booked and nothing filed has no next step, and that is the finding")
	}
}

func TestABookedMeetingIsANextStepHoweverFarOut(t *testing.T) {
	// The horizon belongs to the surface that asks how soon to prepare. Here a
	// meeting at any distance means somebody already agreed what happens next.
	if nextstep.Missing(nextstep.Facts{DealOpen: true, MeetingBooked: true}) {
		t.Fatal("a booked meeting is the next step, whatever its date")
	}
}

func TestAHumanFiledTaskIsANextStep(t *testing.T) {
	if nextstep.Missing(nextstep.Facts{DealOpen: true, OpenHumanTasks: 1}) {
		t.Fatal("work a colleague filed is the agreed next step")
	}
}

func TestAClosedDealIsNeverMissingANextStep(t *testing.T) {
	// Asked first, and asked separately: a won or lost deal has nothing left to
	// agree, so the gap this reports cannot exist on one.
	for _, f := range []nextstep.Facts{
		{},
		{MeetingBooked: true},
		{OpenHumanTasks: 2},
	} {
		if nextstep.Missing(f) {
			t.Fatalf("a closed deal has no next step to miss: %+v", f)
		}
	}
}

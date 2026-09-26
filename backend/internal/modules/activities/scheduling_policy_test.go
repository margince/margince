// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"testing"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/httperr"
)

func TestMeetingGridUsesHostWallTimeIndependentlyOfDuration(t *testing.T) {
	zone, err := time.LoadLocation("Asia/Kathmandu")
	if err != nil {
		t.Fatal(err)
	}
	hours := WorkingHours{StartMinute: 9 * 60, EndMinute: 11 * 60, Days: []int{1}, Location: zone}
	from := time.Date(2026, 10, 5, 9, 0, 0, 0, zone)
	slots, truncated := policySlots(from, from.Add(2*time.Hour), 45*time.Minute, nil, hours)
	if truncated || len(slots) != 6 {
		t.Fatalf("slots=%v truncated=%v", slots, truncated)
	}
	for i, slot := range slots {
		want := from.Add(time.Duration(i) * 15 * time.Minute)
		if !slot.Start.Equal(want) || slot.End.Sub(slot.Start) != 45*time.Minute {
			t.Fatalf("slot %d = %v", i, slot)
		}
	}
}

func TestMeetingGridHonorsBusyIntervalsAndExclusiveEnds(t *testing.T) {
	hours := fallbackWorkingHours()
	from := monday(9)
	slots, _ := policySlots(from, from.Add(2*time.Hour), 30*time.Minute, []slot{{Start: from.Add(30 * time.Minute), End: from.Add(90 * time.Minute)}}, hours)
	if len(slots) != 2 || !slots[0].Start.Equal(from) || !slots[1].Start.Equal(from.Add(90*time.Minute)) {
		t.Fatalf("slots=%v", slots)
	}
}

func TestMeetingGridDoesNotRepeatInstantsAcrossAutumnClockChange(t *testing.T) {
	zone, err := time.LoadLocation("Europe/Berlin")
	if err != nil {
		t.Fatal(err)
	}
	hours := WorkingHours{StartMinute: 60, EndMinute: 240, Days: []int{7}, Location: zone}
	from := time.Date(2026, 10, 25, 1, 0, 0, 0, zone)
	slots, _ := policySlots(from, from.Add(4*time.Hour), 30*time.Minute, nil, hours)
	seen := map[time.Time]bool{}
	for _, slot := range slots {
		if seen[slot.Start] {
			t.Fatalf("repeated instant %v", slot.Start)
		}
		seen[slot.Start] = true
		if slot.End.Sub(slot.Start) != 30*time.Minute {
			t.Fatalf("wrong elapsed duration: %v", slot)
		}
	}
	if len(slots) == 0 {
		t.Fatal("clock change suppressed all available times")
	}
}

func TestEveryRequiredMeetingBodyIDIsNamedWhenAbsent(t *testing.T) {
	err := validateInvitation(crmcontracts.MeetingInvitationRequest{AttendeeEmail: "guest@example.test", Subject: "Discovery"})
	fault, _ := httperr.Classify(err)
	if fault.Status != 422 || len(fault.Fields) != 1 || fault.Fields[0].Field != "contact_id" || fault.Fields[0].Code != "required" {
		t.Fatalf("missing contact was not identified: %+v", fault)
	}
}

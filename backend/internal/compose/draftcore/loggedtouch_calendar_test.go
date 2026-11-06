// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package draftcore

import (
	"testing"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/convstate"
)

func TestDatedNoteSilenceKeepsItsStatedDay(t *testing.T) {
	zone, err := time.LoadLocation("Pacific/Kiritimati")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 6, 12, 9, 0, 0, 0, zone)
	notes := []crmcontracts.Activity{{
		Kind:              crmcontracts.ActivityKindNote,
		OccurredAt:        time.Date(2026, 6, 10, 12, 0, 0, 0, time.UTC),
		OccurredPrecision: "day",
	}}
	state := ClassifyWithLogged(now, zone, time.Time{}, time.Time{}, notes)
	if state.SilenceDays != 2 {
		t.Fatalf("silence = %d, want 2 from the stated date", state.SilenceDays)
	}
}

func TestLoggedTouchOrderFollowsTheRecordCalendar(t *testing.T) {
	zone, err := time.LoadLocation("Pacific/Kiritimati")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 6, 12, 9, 0, 0, 0, zone)
	notes := []crmcontracts.Activity{
		{Kind: crmcontracts.ActivityKindNote, OccurredAt: time.Date(2026, 6, 10, 12, 0, 0, 0, time.UTC), OccurredPrecision: "day"},
		{Kind: crmcontracts.ActivityKindNote, OccurredAt: time.Date(2026, 6, 10, 11, 0, 0, 0, time.UTC), OccurredPrecision: "instant"},
	}
	state := ClassifyWithLogged(now, zone, time.Time{}, time.Time{}, notes)
	if state.SilenceDays != 1 {
		t.Fatalf("silence = %d, want 1 from the latest local activity day", state.SilenceDays)
	}
}

// A past meeting nobody called off is prior contact, whether or not anyone
// reported it held. A meeting still to come, a canceled one and a no-show are not.
func TestAPastMeetingNobodyCalledOffIsPriorContact(t *testing.T) {
	now := time.Date(2026, 6, 12, 9, 0, 0, 0, time.UTC)
	booked, held := crmcontracts.ActivityMeetingStatusBooked, crmcontracts.ActivityMeetingStatusHeld
	canceled, noShow := crmcontracts.ActivityMeetingStatusCanceled, crmcontracts.ActivityMeetingStatusNoShow
	past, ahead := now.AddDate(0, 0, -3), now.AddDate(0, 0, 3)
	for name, tc := range map[string]struct {
		status *crmcontracts.ActivityMeetingStatus
		at     time.Time
		want   bool
	}{
		"no status":     {nil, past, true},
		"booked":        {&booked, past, true},
		"held":          {&held, past, true},
		"still to come": {&booked, ahead, false},
		"canceled":      {&canceled, past, false},
		"a no-show":     {&noShow, past, false},
	} {
		meeting := crmcontracts.Activity{
			Kind: crmcontracts.ActivityKindMeeting, MeetingStatus: tc.status,
			OccurredAt: tc.at, OccurredPrecision: "instant",
		}
		if got := IsLoggedTouch(meeting, now, time.UTC); got != tc.want {
			t.Errorf("%s: the meeting is a logged touch = %v, want %v", name, got, tc.want)
		}
		state := ClassifyWithLogged(now, time.UTC, time.Time{}, time.Time{}, []crmcontracts.Activity{meeting})
		if met := state.Band != convstate.BandNone; met != tc.want {
			t.Errorf("%s: the meeting is prior contact = %v (state %+v), want %v", name, met, state, tc.want)
		}
	}
}

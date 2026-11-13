// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"testing"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/values"
)

func TestTheTaskClockGovernsItsLegacyInstantAndFollowingReminder(t *testing.T) {
	in := LogActivityInput{Kind: "task", DueHasTime: true, DueIntent: &crmcontracts.IntentTime{WallClock: "2026-10-25T02:30", Timezone: "Europe/Berlin"}}
	if err := in.admitPickedTimes(); err != nil {
		t.Fatal(err)
	}
	if err := admitTaskTime(&in); err != nil {
		t.Fatal(err)
	}
	expected := time.Date(2026, 10, 25, 0, 30, 0, 0, time.UTC)
	if in.DueAt == nil || !in.DueAt.Equal(expected) || in.RemindAt == nil || !in.RemindAt.Equal(expected) || in.RemindIntent != in.DueIntent {
		t.Fatalf("task schedule = %+v", in)
	}
}

func TestAnInvalidTaskClockIsRefusedBeforeItReplacesTheInstant(t *testing.T) {
	old := time.Date(2026, 10, 25, 0, 30, 0, 0, time.UTC)
	in := UpdateActivityInput{DueAt: &old, DueIntent: &crmcontracts.IntentTime{WallClock: "2026-02-30T09:00", Timezone: "Europe/Berlin"}}
	if err := in.admitPickedTimes("task"); err == nil {
		t.Fatal("invalid local date admitted")
	}
	if !in.DueAt.Equal(old) {
		t.Fatal("invalid intent replaced the earlier instant")
	}
}

func TestAChosenMeetingStartIsRefusedOnAWholeDayMeetingOrAnotherKind(t *testing.T) {
	chosen := &crmcontracts.IntentTime{WallClock: "2026-10-05T11:00", Timezone: "Europe/Berlin"}
	day := crmcontracts.OccurredPrecisionOccurredPrecisionDay
	for name, req := range map[string]crmcontracts.CreateActivityRequest{
		"a note":              {Kind: crmcontracts.CreateActivityRequestKindCreateActivityRequestKindNote, Source: "manual", Body: new("Notes"), MeetingIntent: chosen},
		"a whole-day meeting": {Kind: crmcontracts.CreateActivityRequestKindCreateActivityRequestKindMeeting, Source: "manual", Subject: new("Workshop"), OccurredPrecision: &day, MeetingIntent: chosen},
	} {
		if _, err := logActivityInput(req, provenanceAdmission{}); err == nil {
			t.Errorf("%s took a chosen meeting start", name)
		}
	}
	in, err := logActivityInput(crmcontracts.CreateActivityRequest{Kind: crmcontracts.CreateActivityRequestKindCreateActivityRequestKindMeeting, Source: "manual", Subject: new("Workshop"), MeetingIntent: chosen}, provenanceAdmission{})
	if err != nil || in.OccurredAt == nil || !in.OccurredAt.Equal(time.Date(2026, 10, 5, 9, 0, 0, 0, time.UTC)) {
		t.Fatalf("meeting start = %v err=%v", in.OccurredAt, err)
	}
}

func TestAChosenDueClockMakesATimedDeadlineAndADateOnlyOneStaysDateOnly(t *testing.T) {
	timed := LogActivityInput{Kind: "task", DueIntent: &crmcontracts.IntentTime{WallClock: "2026-10-05T09:00", Timezone: "Europe/Berlin"}}
	if err := timed.admitPickedTimes(); err != nil {
		t.Fatal(err)
	}
	if err := admitTaskTime(&timed); err != nil {
		t.Fatal(err)
	}
	if !timed.DueHasTime || timed.RemindAt == nil {
		t.Fatalf("a timed clock left the task date-only: has_time=%v remind=%v", timed.DueHasTime, timed.RemindAt)
	}
	dateOnly := LogActivityInput{Kind: "task", DueIntent: &crmcontracts.IntentTime{WallClock: "2026-10-05T23:59:59.000000", Timezone: "Europe/Berlin"}}
	if err := dateOnly.admitPickedTimes(); err != nil || dateOnly.DueHasTime {
		t.Fatalf("a date-only clock became timed: has_time=%v err=%v", dateOnly.DueHasTime, err)
	}

	clock := &crmcontracts.IntentTime{WallClock: "2026-10-05T09:00", Timezone: "Europe/Berlin"}
	patch := UpdateActivityInput{DueIntent: clock}
	if err := patch.admitPickedTimes("task"); err != nil || patch.DueHasTime == nil || !*patch.DueHasTime {
		t.Fatalf("a patched clock did not imply a timed deadline: %v err=%v", patch.DueHasTime, err)
	}
	contradiction := UpdateActivityInput{DueIntent: clock, DueHasTime: new(false)}
	if err := contradiction.admitPickedTimes("task"); err == nil {
		t.Fatal("a timed clock beside due_has_time false was admitted")
	}
}

// The agent surface tells a model to write a date-only deadline with this clock.
// The server reads that clock as no time, and any clock near it as a time.
func TestTheDayOnlyClockTheAgentRuleNamesIsTheOneTheServerReadsAsNoTime(t *testing.T) {
	for clock, timed := range map[string]bool{"T" + values.DayOnlyClock: false, "T23:59": true, "T23:58:59": true} {
		in := LogActivityInput{Kind: "task", DueIntent: &crmcontracts.IntentTime{WallClock: "2026-11-20" + clock, Timezone: "Europe/Berlin"}}
		if err := in.admitPickedTimes(); err != nil || in.DueHasTime != timed {
			t.Errorf("clock %s: has_time=%v, want %v (err %v)", clock, in.DueHasTime, timed, err)
		}
	}
}

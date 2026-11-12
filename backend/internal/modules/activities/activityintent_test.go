// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"testing"
	"time"
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
	if err := in.admitPickedTimes(); err == nil {
		t.Fatal("invalid local date admitted")
	}
	if !in.DueAt.Equal(old) {
		t.Fatal("invalid intent replaced the earlier instant")
	}
}

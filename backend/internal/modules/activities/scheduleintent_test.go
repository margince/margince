// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"errors"
	"testing"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
)

func TestTheChosenLocalScheduleGovernsItsCompatibleInstantFields(t *testing.T) {
	cache := time.Date(2035, 1, 1, 9, 0, 0, 0, time.UTC)
	oldZone := "UTC"
	schedule, err := ParseSendSchedule(&cache, &oldZone, &crmcontracts.IntentTime{WallClock: "2026-10-25T02:30", Timezone: "Europe/Berlin"})
	if err != nil {
		t.Fatal(err)
	}
	expected := time.Date(2026, 10, 25, 0, 30, 0, 0, time.UTC)
	if schedule == nil || !schedule.At.Equal(expected) || schedule.TZ != "Europe/Berlin" {
		t.Fatalf("local choice lost: %+v", schedule)
	}
	wire := schedule.IntentWire()
	if wire == nil || wire.WallClock != "2026-10-25T02:30:00" || wire.Timezone != "Europe/Berlin" {
		t.Fatalf("wire lost local choice: %+v", wire)
	}
	wall := time.Date(2026, 10, 25, 2, 30, 0, 0, time.UTC)
	read, err := readSchedule(&expected, &schedule.TZ, &wall)
	if err != nil {
		t.Fatal(err)
	}
	if read.Intent == nil || read.Intent.WallClock() != wire.WallClock || !read.At.Equal(expected) {
		t.Fatalf("saved local choice did not round-trip: %+v", read)
	}
}

func TestALegacyScheduleNeverGetsAGuessedLocalTime(t *testing.T) {
	at := time.Date(2026, 10, 25, 1, 30, 0, 0, time.UTC)
	zone := "Europe/Berlin"
	schedule, err := ParseSendSchedule(&at, &zone, nil)
	if err != nil {
		t.Fatal(err)
	}
	if schedule == nil || schedule.Intent != nil || !schedule.At.Equal(at) || schedule.IntentWire() != nil {
		t.Fatalf("legacy instant was reinterpreted: %+v", schedule)
	}
	read, err := readSchedule(&at, &zone, nil)
	if err != nil {
		t.Fatal(err)
	}
	if read.Intent != nil || read.wallClock() != nil {
		t.Fatal("legacy read invented a local choice")
	}
	now, err := ParseSendSchedule(nil, nil, nil)
	if err != nil || now != nil {
		t.Fatalf("missing schedule must mean send now: %+v %v", now, err)
	}
}

func TestAnInvalidLocalScheduleNamesTheFieldToCorrect(t *testing.T) {
	for _, c := range []struct{ Wall, Zone, Field string }{
		{"2026-10-25T02:30Z", "Europe/Berlin", "scheduled_intent.wall_clock"},
		{"2026-10-25T02:30", "Local", "scheduled_intent.timezone"},
	} {
		_, err := ParseSendSchedule(nil, nil, &crmcontracts.IntentTime{WallClock: c.Wall, Timezone: c.Zone})
		var invalid *InvalidScheduleError
		if !errors.As(err, &invalid) || invalid.Field != c.Field {
			t.Errorf("got %v, want field %s", err, c.Field)
		}
	}
}

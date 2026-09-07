// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

// The defect these hours exist to remove, stated as the cases that used to be
// wrong.
//
// "9am" was read off a UTC instant, so it meant 4pm in Ho Chi Minh City — and
// the weekend check was wrong the same way, because a Monday morning in Saigon
// is still Sunday to a UTC clock.

import (
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// saigon is the zone the whole class of defects is most visible in: +07, so a
// working day there begins while the previous UTC day is still running.
func saigon(t *testing.T) *time.Location {
	t.Helper()
	loc, err := time.LoadLocation("Asia/Ho_Chi_Minh")
	if err != nil {
		t.Fatalf("the zone database does not carry Asia/Ho_Chi_Minh: %v", err)
	}
	return loc
}

func hoursIn(loc *time.Location, start, end int, days ...int) WorkingHours {
	return WorkingHours{StartMinute: start, EndMinute: end, Days: days, Location: loc}
}

func TestAHostsMorningIsTheirOwnMorningNotUTCsAt(t *testing.T) {
	loc := saigon(t)
	// 02:00Z on a Monday is 09:00 in Saigon — the first slot of that host's
	// week, and a slot the UTC reading refused twice over: too early in the
	// day, and on a Sunday.
	monday := time.Date(2026, 6, 8, 2, 0, 0, 0, time.UTC)
	if monday.UTC().Weekday() != time.Monday {
		t.Fatalf("the fixture is not a Monday: %s", monday)
	}
	free, _ := freeSlots(monday, monday.Add(time.Hour), 30*time.Minute, nil,
		hoursIn(loc, 9*60, 17*60, 1, 2, 3, 4, 5))
	if len(free) == 0 {
		t.Fatal("a host in Saigon is not bookable at nine in the morning — which is the whole defect")
	}
	if !free[0].Start.Equal(monday) {
		t.Errorf("the first slot is %s, want the top of their working day", free[0].Start)
	}
}

func TestSundayEveningInUTCIsAlreadyMondayForThem(t *testing.T) {
	loc := saigon(t)
	// An early starter — 06:00 to 14:00, which is exactly the kind of week one
	// pair of numbers set by an admin gets wrong. 23:00Z on Sunday is 06:00
	// Monday for them, and the UTC reading refused it for being a Sunday.
	sundayEvening := time.Date(2026, 6, 7, 23, 0, 0, 0, time.UTC)
	if sundayEvening.Weekday() != time.Sunday {
		t.Fatalf("the fixture is not a Sunday: %s", sundayEvening)
	}
	free, _ := freeSlots(sundayEvening, sundayEvening.Add(2*time.Hour), 30*time.Minute, nil,
		hoursIn(loc, 6*60, 14*60, 1, 2, 3, 4, 5))
	if len(free) == 0 {
		t.Error("a Monday morning in Saigon was refused for being a Sunday somewhere else")
	}
}

func TestADayTheHostDoesNotWorkOffersNothing(t *testing.T) {
	// A four-day week: Monday to Thursday, which is one of the two cases that
	// prompted the setting.
	friday := time.Date(2026, 6, 12, 10, 0, 0, 0, time.UTC)
	if friday.Weekday() != time.Friday {
		t.Fatalf("the fixture is not a Friday: %s", friday)
	}
	free, _ := freeSlots(friday, friday.Add(4*time.Hour), 30*time.Minute, nil,
		hoursIn(time.UTC, 9*60, 17*60, 1, 2, 3, 4))
	if len(free) != 0 {
		t.Errorf("a host who does not work Fridays was offered %d slot(s)", len(free))
	}
}

func TestAWorkingDayMayRunToMidnight(t *testing.T) {
	// 24:00 is the honest spelling of "until midnight", and 23:59 is a
	// different answer: the last half hour is bookable or it is not.
	lateEvening := time.Date(2026, 6, 8, 23, 0, 0, 0, time.UTC)
	free, _ := freeSlots(lateEvening, lateEvening.Add(time.Hour), 30*time.Minute, nil,
		hoursIn(time.UTC, 18*60, 24*60, 1, 2, 3, 4, 5))
	if len(free) != 2 {
		t.Fatalf("a day running to midnight offered %d slots, want both halves of the last hour", len(free))
	}
	if last := free[len(free)-1].End; last.Hour() != 0 || last.Minute() != 0 {
		t.Errorf("the last slot ends at %s, want midnight", last)
	}
}

func TestASlotCrossingMidnightIsNotInsideTheDayItStartedIn(t *testing.T) {
	// The day ends at 24:00 and the slot would end at 00:30 the next morning.
	// Comparing its end against the FOLLOWING day's minutes would admit it.
	lateEvening := time.Date(2026, 6, 8, 23, 45, 0, 0, time.UTC)
	free, _ := freeSlots(lateEvening, lateEvening.Add(time.Hour), 30*time.Minute, nil,
		hoursIn(time.UTC, 18*60, 24*60, 1, 2, 3, 4, 5))
	for _, slot := range free {
		if slot.End.Day() != slot.Start.Day() {
			t.Errorf("a slot from %s to %s crosses midnight and was offered anyway", slot.Start, slot.End)
		}
	}
}

func TestAnUnwiredStoreKeepsTheStatedFallback(t *testing.T) {
	// The public booking page reaches this path, so a wiring gap must degrade
	// rather than refuse — and it must degrade to something stated.
	var store Store
	hours := store.hoursOf(t.Context(), ids.UserID{})
	want := fallbackWorkingHours()
	if hours.StartMinute != want.StartMinute || hours.EndMinute != want.EndMinute ||
		len(hours.Days) != len(want.Days) || hours.Location != want.Location {
		t.Errorf("an unwired store answers %+v, want the stated fallback %+v", hours, want)
	}
}

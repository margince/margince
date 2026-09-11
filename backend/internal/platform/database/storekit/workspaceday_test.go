// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package storekit_test

import (
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
)

// The calendar day an instant falls on is the installation's, not UTC's: a seat
// east of UTC closing at 01:30 local is on today, not yesterday.
func TestWorkspaceDayTakesTheLocalDayEastOfUTC(t *testing.T) {
	saigon, err := time.LoadLocation("Asia/Ho_Chi_Minh")
	if err != nil {
		t.Fatalf("load zone: %v", err)
	}
	// 18:30Z is 01:30 the next morning in a +7 zone.
	instant := time.Date(2026, 3, 15, 18, 30, 0, 0, time.UTC)

	day := storekit.WorkspaceDay(instant, saigon)

	if y, m, d := day.Date(); y != 2026 || m != time.March || d != 16 {
		t.Fatalf("workspace day = %04d-%02d-%02d, want 2026-03-16", y, m, d)
	}
}

// And west of UTC the same instant is the previous day.
func TestWorkspaceDayTakesTheLocalDayWestOfUTC(t *testing.T) {
	newYork, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Fatalf("load zone: %v", err)
	}
	// 02:30Z is 22:30 the previous evening in New York (EDT, -4, in March).
	instant := time.Date(2026, 3, 15, 2, 30, 0, 0, time.UTC)

	day := storekit.WorkspaceDay(instant, newYork)

	if y, m, d := day.Date(); y != 2026 || m != time.March || d != 14 {
		t.Fatalf("workspace day = %04d-%02d-%02d, want 2026-03-14", y, m, d)
	}
}

// The result is a DATE, not an instant: local Y-M-D re-anchored at UTC midnight,
// which is the shape a `date` column or a `::date` bind reads back unchanged. A
// value carrying the local wall time would bind as a different day west of UTC.
func TestWorkspaceDayIsADateAtUTCMidnight(t *testing.T) {
	saigon, err := time.LoadLocation("Asia/Ho_Chi_Minh")
	if err != nil {
		t.Fatalf("load zone: %v", err)
	}
	day := storekit.WorkspaceDay(time.Date(2026, 3, 15, 18, 30, 0, 0, time.UTC), saigon)

	if loc := day.Location(); loc != time.UTC {
		t.Fatalf("location = %v, want UTC", loc)
	}
	if h, m, s := day.Clock(); h != 0 || m != 0 || s != 0 {
		t.Fatalf("clock = %02d:%02d:%02d, want 00:00:00", h, m, s)
	}
}

// AsDate takes the calendar day a value ALREADY names — its own year-month-
// day — and must not project through any zone. A date the admin chose as
// 2026-03-16 stays the 16th, where WorkspaceDay of that same UTC-midnight instant
// west of UTC would slip it to the 15th. Two different questions, two primitives.
func TestAsDateKeepsTheChosenDayWithoutZoneShift(t *testing.T) {
	chosen := time.Date(2026, 3, 16, 0, 0, 0, 0, time.UTC)

	day := storekit.AsDate(chosen)

	if y, m, d := day.Date(); y != 2026 || m != time.March || d != 16 {
		t.Fatalf("plain date = %04d-%02d-%02d, want 2026-03-16", y, m, d)
	}
	if loc := day.Location(); loc != time.UTC {
		t.Fatalf("location = %v, want UTC", loc)
	}
}

// A value carrying a wall time is reduced to its date, so a sub-day offset can
// never store a calendar day different from the one it names.
func TestAsDateDropsTheTimeOfDay(t *testing.T) {
	day := storekit.AsDate(time.Date(2026, 3, 16, 14, 30, 0, 0, time.UTC))

	if h, m, s := day.Clock(); h != 0 || m != 0 || s != 0 {
		t.Fatalf("clock = %02d:%02d:%02d, want 00:00:00", h, m, s)
	}
}

// The day boundary is asked directly rather than truncated, because in some
// zones local midnight does not exist. Havana springs forward AT midnight on
// 2026-03-08, so that day's first instant is 01:00 local — Truncate/Add would
// land an hour before the day it bounds has even begun.
func TestStartOfNextDayIsExactWhereMidnightDoesNotExist(t *testing.T) {
	havana, err := time.LoadLocation("America/Havana")
	if err != nil {
		t.Fatalf("load zone: %v", err)
	}
	// Noon on the 7th: unambiguous, and the next day is the one that skips 00:00.
	asOf := time.Date(2026, 3, 7, 12, 0, 0, 0, havana)

	start := storekit.StartOfNextDay(asOf, havana)

	local := start.In(havana)
	if y, m, d := local.Date(); y != 2026 || m != time.March || d != 8 {
		t.Fatalf("start date = %04d-%02d-%02d, want 2026-03-08", y, m, d)
	}
	if h := local.Hour(); h != 1 {
		t.Fatalf("start hour = %02d, want 01 (00:00 does not exist that day)", h)
	}
	// It is the FIRST such instant: a minute earlier is still the 7th.
	if before := start.Add(-time.Minute).In(havana); before.Day() != 7 {
		t.Fatalf("a minute before start fell on day %d, want the 7th", before.Day())
	}
}

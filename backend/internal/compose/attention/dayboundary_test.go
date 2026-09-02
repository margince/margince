// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package attention

// Where today ends for the due-dated lanes.

import (
	"context"
	"testing"
	"time"
)

func zoneNamed(t *testing.T, name string) Zone {
	t.Helper()
	loc, err := time.LoadLocation(name)
	if err != nil {
		t.Fatalf("loading %s: %v", name, err)
	}
	return func(context.Context) (*time.Location, error) { return loc, nil }
}

// THE DAY ENDS AT THE INSTALLATION'S MIDNIGHT, not UTC's.
//
// Truncating the clock to 24 hours answers UTC midnight wherever the
// installation is: a UTC+7 seat's "today" ran through 07:00 tomorrow local, and
// a UTC-4 seat's stopped at 20:00 — so one saw work it is not owed yet and the
// other lost the evening's.
func TestTheDayEndsAtTheInstallationsMidnight(t *testing.T) {
	// Mid-afternoon in Ho Chi Minh City, which is 08:30 UTC.
	asOf := time.Date(2026, 6, 15, 8, 30, 0, 0, time.UTC)
	for name, tc := range map[string]struct {
		zone string
		want time.Time
	}{
		"east of UTC": {"Asia/Ho_Chi_Minh", time.Date(2026, 6, 16, 0, 0, 0, 0, time.UTC).Add(-7 * time.Hour)},
		"west of UTC": {"America/New_York", time.Date(2026, 6, 16, 0, 0, 0, 0, time.UTC).Add(4 * time.Hour)},
		"at UTC":      {"UTC", time.Date(2026, 6, 16, 0, 0, 0, 0, time.UTC)},
	} {
		t.Run(name, func(t *testing.T) {
			s := &Service{zone: zoneNamed(t, tc.zone)}
			got, err := s.endOfDay(context.Background(), asOf)
			if err != nil {
				t.Fatalf("endOfDay: %v", err)
			}
			if !got.Equal(tc.want) {
				t.Errorf("today ends at %s, want %s — the boundary is the installation's "+
					"midnight, and %s reads it as UTC's", got.UTC(), tc.want.UTC(), name)
			}
		})
	}
}

// AND A DAY THAT IS NOT 24 HOURS LONG still ends at midnight.
//
// Clocks move twice a year, and adding a fixed span lands an hour off on those
// mornings — in the direction that drops work the reader is owed.
func TestADayTheClocksChangedInStillEndsAtMidnight(t *testing.T) {
	berlin := zoneNamed(t, "Europe/Berlin")
	for name, asOf := range map[string]time.Time{
		// 2026-03-29 is a 23-hour day in Berlin; 2026-10-25 is 25 hours.
		"the short day": time.Date(2026, 3, 29, 10, 0, 0, 0, time.UTC),
		"the long day":  time.Date(2026, 10, 25, 10, 0, 0, 0, time.UTC),
	} {
		t.Run(name, func(t *testing.T) {
			s := &Service{zone: berlin}
			got, err := s.endOfDay(context.Background(), asOf)
			if err != nil {
				t.Fatalf("endOfDay: %v", err)
			}
			loc, _ := time.LoadLocation("Europe/Berlin")
			local := got.In(loc)
			if local.Hour() != 0 || local.Minute() != 0 {
				t.Errorf("today ends at %s local, want midnight — a fixed 24 hours misses it on "+
					"the two mornings a year the clocks move", local)
			}
			if !local.After(asOf.In(loc)) {
				t.Errorf("the boundary %s is not after the instant %s it was asked about", local, asOf.In(loc))
			}
		})
	}
}

// AN UNBOUND ZONE IS UTC, which is what every composition with no installation
// to ask gets — and is why the shipped wiring has to bind one.
func TestAnUnboundZoneIsUTC(t *testing.T) {
	asOf := time.Date(2026, 6, 15, 8, 30, 0, 0, time.UTC)
	got, err := (&Service{}).endOfDay(context.Background(), asOf)
	if err != nil {
		t.Fatalf("endOfDay: %v", err)
	}
	if want := time.Date(2026, 6, 16, 0, 0, 0, 0, time.UTC); !got.Equal(want) {
		t.Errorf("endOfDay = %s, want %s", got, want)
	}
}

// AND A ZONE THAT WILL NOT RESOLVE stops the lane rather than serving UTC's day
// under the installation's name.
func TestAZoneThatWillNotResolveIsAnError(t *testing.T) {
	boom := func(context.Context) (*time.Location, error) { return nil, context.DeadlineExceeded }
	if _, err := (&Service{zone: boom}).endOfDay(context.Background(), time.Now()); err == nil {
		t.Error("a zone the installation could not answer for produced a boundary anyway")
	}
}

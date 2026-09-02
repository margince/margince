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

// THE INSTALLATION IS ASKED ONCE PER ASSEMBLY, not once per lane.
//
// Three due-dated lanes read the boundary. Resolving per lane means an operator
// moving the installation mid-read gives one lane yesterday's midnight and the
// next one today's, inside a single response — and each resolution is a
// transaction, so it also pays three times for one fact.
func TestTheZoneIsResolvedOncePerAssembly(t *testing.T) {
	var asked int
	loc, err := time.LoadLocation("Asia/Ho_Chi_Minh")
	if err != nil {
		t.Fatalf("loading the zone: %v", err)
	}
	s := NewService(
		stubApprovals{}, stubDuplicates{}, &stubTasks{}, stubReceipts{},
		stubBriefing{}, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, fixedClock,
		WithZone(func(context.Context) (*time.Location, error) { asked++; return loc, nil }),
	)
	if _, err := s.Assemble(context.Background()); err != nil {
		t.Fatalf("assembling: %v", err)
	}
	if asked != 1 {
		t.Errorf("the installation was asked for its timezone %d times, want once — one response "+
			"must not judge two lanes against two different days", asked)
	}
}

// AND A SEAM THAT ANSWERS NOTHING WITHOUT FAILING is a broken binding, not a
// zone: reaching time.In with a nil location panics, and a feed that panics
// tells an operator less than one that reports.
func TestAZoneSeamAnsweringNoLocationIsAnErrorNotAPanic(t *testing.T) {
	// The very shape nilnil refuses in production code, written on purpose: what
	// this pins is that the feed survives a binding somebody got wrong.
	//nolint:nilnil // the malformed seam IS the subject
	nothing := func(context.Context) (*time.Location, error) { return nil, nil }
	if _, err := (&Service{zone: nothing}).endOfDay(context.Background(), time.Now()); err == nil {
		t.Error("a nil location produced a boundary; the next call would have panicked instead")
	}
}

// A DAY WHOSE MIDNIGHT DOES NOT EXIST still ends when it ends.
//
// Some zones spring forward AT midnight, so 00:00 is not a time that day. Go's
// normalisation then answers a neighbouring instant, and which way it leans is
// not the same everywhere: Havana and Santiago normalise BACKWARD, to 23:00 on
// the previous date — an hour before the day being bounded has ended, so the
// last hour's work falls off the lane. Beirut leans forward, to 01:00, and is
// right by luck rather than by rule.
//
// The dates are FOUND rather than written down. A midnight transition moves when
// a country changes its mind, and tzdata follows; a test naming 2026-03-08 would
// go red on the machine whose tzdata moved it, over a property the code still
// holds. So this scans for the days that have no local midnight and asserts the
// rule on each — and fails only if NO zone has one anywhere in the window, which
// is the state in which it would be proving nothing.
func TestADayWhoseMidnightDoesNotExistEndsAtItsFirstInstant(t *testing.T) {
	zones := []string{"America/Havana", "America/Santiago", "Asia/Beirut"}
	var found int
	for _, name := range zones {
		loc, err := time.LoadLocation(name)
		if err != nil {
			// A zone this tzdata does not carry is not a failure of the rule.
			t.Logf("%s is not in this tzdata; skipping it", name)
			continue
		}
		for _, day := range daysWithNoLocalMidnight(loc) {
			found++
			t.Run(name+"/"+day.Format(time.DateOnly), func(t *testing.T) {
				// Asked from mid-afternoon the day BEFORE, so the boundary
				// under test is that day's own end.
				asOf := day.AddDate(0, 0, -1).Add(15 * time.Hour)
				local := startOfNextDay(asOf.In(loc), loc).In(loc)
				if !sameDate(local, day) {
					t.Fatalf("the day ends at %s, which is not the start of %s", local, day.Format(time.DateOnly))
				}
				if before := local.Add(-time.Minute); sameDate(before, local) {
					t.Errorf("%s is not the FIRST instant of %s — a minute earlier is still the same "+
						"date, so the boundary cuts the previous day short", local, day.Format(time.DateOnly))
				}
			})
		}
	}
	if found == 0 {
		t.Fatalf("no day without a local midnight in %v under this tzdata, so this test proves nothing "+
			"— find a zone that still springs forward at midnight and name it here", zones)
	}
}

// daysWithNoLocalMidnight returns the local dates in a two-year window whose
// 00:00 does not exist in loc, each as midday of that date so the value itself
// is never the ambiguous instant.
func daysWithNoLocalMidnight(loc *time.Location) []time.Time {
	var out []time.Time
	for cursor := time.Date(2026, 1, 1, 12, 0, 0, 0, loc); cursor.Year() < 2028; cursor = cursor.AddDate(0, 0, 1) {
		year, month, day := cursor.Date()
		if midnight := time.Date(year, month, day, 0, 0, 0, 0, loc); !sameDate(midnight, cursor) || midnight.Hour() != 0 {
			out = append(out, time.Date(year, month, day, 0, 0, 0, 0, time.UTC))
		}
	}
	return out
}

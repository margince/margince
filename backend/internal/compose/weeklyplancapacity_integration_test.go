// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// What next week already holds, over real migrated Postgres.
//
// The window is the whole subject. A capacity line that counted this week, or a
// fixed 168 hours, or a meeting somebody has already held would each be a
// plausible number that answers a different question than the one a rep is
// asking on a Monday — and none of them is visible without a database that
// knows the installation's reporting zone.

import (
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// capacityClock is a Wednesday, so "next week" is unambiguous and is not the
// week the fixture's own writes land in.
var capacityClock = time.Date(2026, 6, 10, 9, 0, 0, 0, time.UTC)

// THE WINDOW IS NEXT WEEK'S, AND ONLY BOOKED COUNTS.
//
// Three meetings, one in each of the weeks either side and one inside. Plus a
// held meeting inside the window, which must not count: the week has not
// happened, so a meeting with an outcome there is one somebody backdated, and
// counting it would inflate a figure a rep plans against.
func TestCapacityCountsOnlyBookedMeetingsInTheNamedWeeksLocalWindow(t *testing.T) {
	e := integration.Setup(t)
	rep := e.As(e.Rep1, []ids.UUID{e.Team1}, integration.AdminPerms)

	// capacityClock is Wednesday 10 June 2026; this week's Monday is the 8th,
	// so next week runs Monday 15 June to Sunday 21 June inclusive.
	// TWO in this week, so the count differs between the two windows. With one
	// in each, a seam reading the wrong week answers 1 either way and the
	// assertion cannot tell them apart — which is exactly how this test first
	// passed with the window shifted a week off.
	bookMeeting(t, e, "This week — too early", capacityClock.AddDate(0, 0, 1), "booked")
	bookMeeting(t, e, "This week — also too early", capacityClock.AddDate(0, 0, 2), "booked")
	bookMeeting(t, e, "Next week — counts", time.Date(2026, 6, 16, 10, 0, 0, 0, time.UTC), "booked")
	bookMeeting(t, e, "Next week but held", time.Date(2026, 6, 17, 10, 0, 0, 0, time.UTC), "held")
	bookMeeting(t, e, "The week after — too late", time.Date(2026, 6, 23, 10, 0, 0, 0, time.UTC), "booked")

	// The shared bookMeeting helper logs as the ADMIN, so the meetings above are
	// attributed to them; asking about Rep1 would count a week nobody booked
	// and pass at zero whatever the window did.
	seam := weeklyPlanCapacity{pool: e.Pool}
	// The week is NAMED by the caller, which is the plan's own
	// local_week_start in production. Derived from the clock instead, the
	// heading, the stored row and this line answered about three different
	// sets of seven days.
	nextWeek := time.Date(2026, 6, 15, 0, 0, 0, 0, time.UTC)
	got, err := seam.ForWeek(rep, e.AdminUser, nextWeek)
	if err != nil {
		t.Fatalf("reading the named week's capacity: %v", err)
	}
	if got.Meetings != 1 {
		t.Fatalf("one booked meeting falls in next week, got %d", got.Meetings)
	}
}

// The seam prices the week it is GIVEN, not the one after today.
//
// The fixture is asymmetric on purpose: two meetings in the current week and
// one in the next. A seam that ignored its argument and computed "next week"
// would answer 1 whichever week it was asked about, and the assertion could not
// tell the two apart — which is how the original defect survived, with the plan
// stored against one week and priced against another.
func TestCapacityPricesTheWeekItIsGiven(t *testing.T) {
	e := integration.Setup(t)
	rep := e.As(e.Rep1, []ids.UUID{e.Team1}, integration.AdminPerms)

	bookMeeting(t, e, "This week — one", capacityClock.AddDate(0, 0, 1), "booked")
	bookMeeting(t, e, "This week — two", capacityClock.AddDate(0, 0, 2), "booked")
	bookMeeting(t, e, "Next week — one", time.Date(2026, 6, 16, 10, 0, 0, 0, time.UTC), "booked")

	seam := weeklyPlanCapacity{pool: e.Pool}
	thisWeek := time.Date(2026, 6, 8, 0, 0, 0, 0, time.UTC)
	nextWeek := time.Date(2026, 6, 15, 0, 0, 0, 0, time.UTC)

	current, err := seam.ForWeek(rep, e.AdminUser, thisWeek)
	if err != nil {
		t.Fatal(err)
	}
	if current.Meetings != 2 {
		t.Errorf("asked about the week of the 8th, counted %d meetings, want its two",
			current.Meetings)
	}
	coming, err := seam.ForWeek(rep, e.AdminUser, nextWeek)
	if err != nil {
		t.Fatal(err)
	}
	if coming.Meetings != 1 {
		t.Errorf("asked about the week of the 15th, counted %d meetings, want its one",
			coming.Meetings)
	}
}

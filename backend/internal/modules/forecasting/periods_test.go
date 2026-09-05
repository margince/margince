// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package forecasting

import (
	"testing"
	"time"
)

func berlin(t *testing.T) *time.Location {
	t.Helper()
	zone, err := time.LoadLocation("Europe/Berlin")
	if err != nil {
		t.Fatalf("loading the test zone: %v", err)
	}
	return zone
}

func TestAQuarterOpensWhereTheFinancialYearSaysItDoes(t *testing.T) {
	t.Parallel()
	zone := berlin(t)

	for _, tc := range []struct {
		name        string
		at          time.Time
		fiscalStart int
		wantStart   string
		wantEnd     string
	}{
		{
			name:        "calendar year, mid-quarter",
			at:          time.Date(2026, time.May, 14, 12, 0, 0, 0, zone),
			fiscalStart: 1,
			wantStart:   "2026-04-01",
			wantEnd:     "2026-06-30",
		},
		{
			// The case the fiscal setting exists for: a year opening in April
			// puts May in the FIRST quarter, not the second.
			name:        "financial year opening in April",
			at:          time.Date(2026, time.May, 14, 12, 0, 0, 0, zone),
			fiscalStart: 4,
			wantStart:   "2026-04-01",
			wantEnd:     "2026-06-30",
		},
		{
			// A date BEFORE the opening month belongs to the financial year
			// that started the previous calendar year — the case the modulo's
			// +12 is there for.
			name:        "before the opening month, so the previous year's Q4",
			at:          time.Date(2026, time.February, 3, 9, 0, 0, 0, zone),
			fiscalStart: 4,
			wantStart:   "2026-01-01",
			wantEnd:     "2026-03-31",
		},
		{
			// February in a leap year. The end day is derived from the next
			// window's start rather than counted, so 29 days is not a case
			// anybody has to have remembered.
			name:        "a leap February",
			at:          time.Date(2028, time.February, 10, 9, 0, 0, 0, zone),
			fiscalStart: 1,
			wantStart:   "2028-01-01",
			wantEnd:     "2028-03-31",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			period, err := ResolvePeriod(PeriodQuarter, tc.at, tc.fiscalStart, zone)
			if err != nil {
				t.Fatalf("resolving the period: %v", err)
			}
			if got := period.StartDate.Format(time.DateOnly); got != tc.wantStart {
				t.Errorf("period opens %s, want %s", got, tc.wantStart)
			}
			if got := period.EndDate.Format(time.DateOnly); got != tc.wantEnd {
				t.Errorf("period closes %s, want %s", got, tc.wantEnd)
			}
			// The last day is IN the window and the exclusive instant bound is
			// the day after it. A window whose two spellings disagree would put
			// a deal in one reading and not the other.
			if !period.ContainsDay(period.EndDate) {
				t.Error("the period's own last day reads as outside it")
			}
			if !period.End.After(period.EndDate) {
				t.Error("the exclusive bound is not after the last day inside")
			}
		})
	}
}

func TestAMonthIsItsOwnCalendarMonth(t *testing.T) {
	t.Parallel()
	zone := berlin(t)
	period, err := ResolvePeriod(PeriodMonth, time.Date(2026, time.May, 14, 12, 0, 0, 0, zone), 4, zone)
	if err != nil {
		t.Fatalf("resolving the period: %v", err)
	}
	// The financial year shifts QUARTERS. A month is a month in every
	// installation, so the fiscal setting must not move it.
	if got := period.StartDate.Format(time.DateOnly); got != "2026-05-01" {
		t.Errorf("month opens %s, want 2026-05-01", got)
	}
	if got := period.EndDate.Format(time.DateOnly); got != "2026-05-31" {
		t.Errorf("month closes %s, want 2026-05-31", got)
	}
}

// The defect the two-spelling Period exists to prevent, written as a test.
//
// A deal closing at 00:30 Berlin on 1 July is 22:30 UTC on 30 June. Read in
// UTC it lands in the quarter that just ended; read in the installation's own
// zone it lands in the new one. Whichever answer is right, both readings have
// to give the SAME one, or the deal leaves the evidence reading without
// entering the won reading and disappears from the headline with no bucket to
// explain it.
func TestAnInstantJustPastLocalMidnightBelongsToTheLocalDay(t *testing.T) {
	t.Parallel()
	zone := berlin(t)

	closedAt := time.Date(2026, time.July, 1, 0, 30, 0, 0, zone)
	if got := closedAt.UTC().Day(); got != 30 {
		t.Fatalf("the fixture is not the case under test: in UTC this instant is day %d, want 30", got)
	}

	ending, err := ResolvePeriod(PeriodQuarter, time.Date(2026, time.June, 15, 12, 0, 0, 0, zone), 1, zone)
	if err != nil {
		t.Fatalf("resolving the ending quarter: %v", err)
	}
	opening, err := ResolvePeriod(PeriodQuarter, time.Date(2026, time.August, 15, 12, 0, 0, 0, zone), 1, zone)
	if err != nil {
		t.Fatalf("resolving the opening quarter: %v", err)
	}

	if ending.ContainsInstant(closedAt) {
		t.Error("a deal closed after local midnight on 1 July counted into the quarter that ended on 30 June")
	}
	if !opening.ContainsInstant(closedAt) {
		t.Error("a deal closed after local midnight on 1 July did not count into the quarter that began that day")
	}
	// The whole point: the expected-close DATE and the close INSTANT for the
	// same deal must land in the same window. A deal expected on 1 July and
	// closed at 00:30 that morning is in one period, by both readings.
	expected := time.Date(2026, time.July, 1, 0, 0, 0, 0, zone)
	if opening.ContainsDay(expected) != opening.ContainsInstant(closedAt) {
		t.Error("the date reading and the instant reading disagree about the same deal, which is how it vanishes from both")
	}
}

func TestAPeriodRefusesInputsItCannotResolve(t *testing.T) {
	t.Parallel()
	zone := berlin(t)
	at := time.Date(2026, time.May, 14, 12, 0, 0, 0, zone)

	if _, err := ResolvePeriod(PeriodQuarter, at, 1, nil); err == nil {
		t.Error("a period resolved with no zone — which day an instant falls on has no answer without one")
	}
	for _, month := range []int{0, 13, -1} {
		if _, err := ResolvePeriod(PeriodQuarter, at, month, zone); err == nil {
			t.Errorf("fiscal start month %d was accepted", month)
		}
	}
	// The admitting case. Without it a resolver that refused EVERY input would
	// pass every assertion above.
	if _, err := ResolvePeriod(PeriodQuarter, at, 4, zone); err != nil {
		t.Errorf("a valid fiscal year was refused: %v", err)
	}
}

// A week runs Monday to Sunday in the installation's own zone.
func TestAWeekPeriodRunsMondayToSundayInTheInstallationZone(t *testing.T) {
	t.Parallel()
	zone, err := time.LoadLocation("Europe/Berlin")
	if err != nil {
		t.Fatal(err)
	}
	monday := time.Date(2026, 6, 1, 0, 0, 0, 0, zone)

	week, err := ResolveWeek(monday, zone)
	if err != nil {
		t.Fatalf("resolving the week of %s: %v", monday.Format("2006-01-02"), err)
	}
	if week.StartDate.Weekday() != time.Monday {
		t.Fatalf("the week opened on a %s", week.StartDate.Weekday())
	}
	if week.EndDate.Weekday() != time.Sunday {
		t.Fatalf("the week closed on a %s", week.EndDate.Weekday())
	}
	if got := week.EndDate.Sub(week.StartDate).Hours() / 24; got != 6 {
		t.Fatalf("the window spans %v days between its inclusive ends, wanted 6", got)
	}
	// The Sunday is IN the week, which is what a closed upper bound means: a
	// deal closing on Sunday belongs to the week somebody worked it.
	if !week.ContainsDay(week.EndDate) {
		t.Fatal("the week does not contain its own last day")
	}
	if week.ContainsDay(week.EndDate.AddDate(0, 0, 1)) {
		t.Fatal("the week contains the Monday after it")
	}
}

// A week crossing a daylight-saving change is still seven local days.
//
// Europe/Berlin springs forward on 2026-03-29, so that week is 167 hours. A
// window built by adding a duration would close an hour early and drop
// whatever happened in the last hour of Sunday.
func TestAWeekAcrossADstChangeIsStillSevenLocalDays(t *testing.T) {
	t.Parallel()
	zone, err := time.LoadLocation("Europe/Berlin")
	if err != nil {
		t.Fatal(err)
	}
	monday := time.Date(2026, 3, 23, 0, 0, 0, 0, zone)

	week, err := ResolveWeek(monday, zone)
	if err != nil {
		t.Fatal(err)
	}
	if got := week.End.Sub(week.Start); got == 7*24*time.Hour {
		t.Fatal("the week spans exactly 168 hours across a spring-forward, " +
			"so it was built by adding a duration rather than seven local days")
	}
	// The last hour of Sunday is still in the week.
	lastHour := time.Date(2026, 3, 29, 23, 30, 0, 0, zone)
	if !week.ContainsInstant(lastHour) {
		t.Fatal("the final evening of a spring-forward week fell outside it")
	}
}

// A week is never derived from anything but the Monday handed in.
//
// Which day a week opens on is weekly.WeekStartOf's answer, derived through the
// installation's reporting zone. A resolver that rounded to the nearest Monday
// would be a second answer, and the two would file a Sunday-night job's work
// under different weeks.
func TestAWeekPeriodIsNeverDerivedFromAnythingButTheMondayHandedIn(t *testing.T) {
	t.Parallel()
	zone, err := time.LoadLocation("Europe/Berlin")
	if err != nil {
		t.Fatal(err)
	}
	for _, bad := range []struct {
		name string
		at   time.Time
	}{
		{"a Wednesday", time.Date(2026, 6, 3, 0, 0, 0, 0, zone)},
		{"the Sunday before", time.Date(2026, 5, 31, 0, 0, 0, 0, zone)},
		{"a Monday afternoon", time.Date(2026, 6, 1, 14, 0, 0, 0, zone)},
	} {
		t.Run(bad.name, func(t *testing.T) {
			if _, err := ResolveWeek(bad.at, zone); err == nil {
				t.Fatal("accepted, so the window it built is not the week weekly.WeekStartOf named")
			}
		})
	}
}

// ResolvePeriod refuses a week rather than treating it as a quarter.
//
// monthsIn returns 3 for anything it does not recognise, so an unrefused week
// would resolve to a three-month window and every reading would silently be
// over the wrong period.
func TestResolvePeriodRefusesAWeekRatherThanReadingItAsAQuarter(t *testing.T) {
	t.Parallel()
	zone, err := time.LoadLocation("Europe/Berlin")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ResolvePeriod(PeriodWeek, time.Date(2026, 6, 3, 9, 0, 0, 0, zone), 1, zone); err == nil {
		t.Fatal("a week resolved through the financial year, which would be a quarter wearing the word week")
	}
}

// A deal expected on the window's own last day is IN the window, whichever side
// of UTC the installation sits.
//
// The two sides carry their midnight in different zones: `expected_close_date`
// is a Postgres `date` and pgx scans it as UTC midnight, while StartDate and
// EndDate are midnight where the installation is. Comparing those as instants
// asks which moment came first, which is not the question — and it answers
// wrongly at exactly the ends this window is inclusive at.
//
// Nine hours east, a deal expected on the last day scans as 09:00 that day and
// reads as after an EndDate of 00:00; west of UTC the same arithmetic drops the
// first day instead. Either way the deal leaves the reading with nothing to say
// where it went.
func TestADateColumnLandsInsideTheWindowOnBothSidesOfUTC(t *testing.T) {
	t.Parallel()
	for _, place := range []struct {
		name string
		zone string
	}{
		{"east of UTC", "Asia/Tokyo"},
		{"west of UTC", "America/Los_Angeles"},
	} {
		t.Run(place.name, func(t *testing.T) {
			zone, err := time.LoadLocation(place.zone)
			if err != nil {
				t.Fatal(err)
			}
			week, err := ResolveWeek(time.Date(2026, 6, 1, 0, 0, 0, 0, zone), zone)
			if err != nil {
				t.Fatal(err)
			}
			// Exactly how pgx hands over a `date`.
			asScanned := func(y int, m time.Month, d int) time.Time {
				return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
			}
			if !week.ContainsDay(asScanned(2026, 6, 1)) {
				t.Error("a deal expected on the week's first day fell outside it")
			}
			if !week.ContainsDay(asScanned(2026, 6, 7)) {
				t.Error("a deal expected on the week's last day fell outside it")
			}
			// And the window still ENDS: the day after must not be admitted, or
			// the fix above would be a comparison that says yes to everything.
			if week.ContainsDay(asScanned(2026, 6, 8)) {
				t.Error("the week admitted the Monday after it")
			}
			if week.ContainsDay(asScanned(2026, 5, 31)) {
				t.Error("the week admitted the Sunday before it")
			}
		})
	}
}

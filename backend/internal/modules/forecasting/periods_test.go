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

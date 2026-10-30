// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/reportperiod"
)

// resolveInTx resolves a rollup period in one workspace transaction, as the
// rollup read does.
func resolveInTx(t *testing.T, e *integration.Env, period CompanyRollupPeriod, asOf time.Time, zone *time.Location, fiscalStart int) (RollupWonPeriod, map[string]string, error) {
	t.Helper()
	var won RollupWonPeriod
	var current map[string]string
	err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		var err error
		if current, err = rollupCurrentPeriods(e.Admin(), tx, asOf, zone, fiscalStart); err != nil {
			return err
		}
		won, err = resolveRollupPeriod(e.Admin(), tx, period, current, asOf, zone, fiscalStart)
		return err
	})
	return won, current, err
}

// With no period named, closed-won sums the fiscal quarter under way in the
// installation's zone. The start months other than January share no boundary
// with the calendar quarters, so a cut that ignored the setting fails them.
func TestTheRollupWithNoPeriodSumsTheFiscalQuarterUnderWay(t *testing.T) {
	e := integration.Setup(t)
	losAngeles, err := time.LoadLocation("America/Los_Angeles")
	if err != nil {
		t.Fatal(err)
	}
	utc := time.UTC
	day := func(y int, m time.Month, d int, loc *time.Location) time.Time {
		return time.Date(y, m, d, 0, 0, 0, 0, loc)
	}
	cases := []struct {
		name        string
		now         time.Time
		loc         *time.Location
		fiscalStart int
		start, end  time.Time
		label       string
	}{
		{"mid-quarter", time.Date(2026, 5, 15, 10, 30, 0, 0, utc), utc, 1, day(2026, 4, 1, utc), day(2026, 7, 1, utc), "2026-Q2"},
		{"the boundary opens its quarter", day(2026, 7, 1, utc), utc, 1, day(2026, 7, 1, utc), day(2026, 10, 1, utc), "2026-Q3"},
		{"the last microsecond closes its quarter", time.Date(2026, 6, 30, 23, 59, 59, 999999000, utc), utc, 1, day(2026, 4, 1, utc), day(2026, 7, 1, utc), "2026-Q2"},
		{"the zone moves the year", time.Date(2026, 1, 1, 4, 0, 0, 0, utc), losAngeles, 1, day(2025, 10, 1, losAngeles), day(2026, 1, 1, losAngeles), "2025-Q4"},
		{"february start, first quarter", time.Date(2026, 3, 15, 10, 30, 0, 0, utc), utc, 2, day(2026, 2, 1, utc), day(2026, 5, 1, utc), "FY2026/27-Q1"},
		{"february start, a quarter reaching back a year", day(2026, 1, 10, utc), utc, 2, day(2025, 11, 1, utc), day(2026, 2, 1, utc), "FY2025/26-Q4"},
		{"february start, its first instant", day(2026, 2, 1, utc), utc, 2, day(2026, 2, 1, utc), day(2026, 5, 1, utc), "FY2026/27-Q1"},
		{"november start, a quarter crossing the year", time.Date(2026, 12, 20, 12, 0, 0, 0, utc), utc, 11, day(2026, 11, 1, utc), day(2027, 2, 1, utc), "FY2026/27-Q1"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			won, current, err := resolveInTx(t, e, CompanyRollupPeriod{}, tc.now, tc.loc, tc.fiscalStart)
			if err != nil {
				t.Fatal(err)
			}
			if !won.Start.Equal(tc.start) || !won.End.Equal(tc.end) {
				t.Errorf("window = [%s, %s), want [%s, %s)", won.Start, won.End, tc.start, tc.end)
			}
			if won.Label != tc.label || won.Grain != reportperiod.GrainQuarter || won.Chosen {
				t.Errorf("period = %s %s chosen=%v, want the unchosen quarter %s", won.Grain, won.Label, won.Chosen, tc.label)
			}
			if current[fieldPeriodQuarter] != tc.label {
				t.Errorf("current quarter = %q, want %q", current[fieldPeriodQuarter], tc.label)
			}
			if won.Next != nil {
				t.Errorf("the quarter after the one under way has not begun, but next = %q", *won.Next)
			}
		})
	}
}

// A named period is read in the spelling the installation's reports render,
// and steps to its neighbours in that spelling.
func TestARolledUpPeriodIsReadInTheInstallationsSpelling(t *testing.T) {
	e := integration.Setup(t)
	zone, err := time.LoadLocation("Asia/Ho_Chi_Minh")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 10, 30, 9, 0, 0, 0, zone)
	won, current, err := resolveInTx(t, e, CompanyRollupPeriod{Field: fieldPeriodQuarter, Label: "FY2025/26-Q4"}, now, zone, 4)
	if err != nil {
		t.Fatal(err)
	}
	if !won.Start.Equal(time.Date(2026, 1, 1, 0, 0, 0, 0, zone)) || !won.End.Equal(time.Date(2026, 4, 1, 0, 0, 0, 0, zone)) || !won.Chosen {
		t.Errorf("FY2025/26-Q4 under an April start = [%s, %s) chosen=%v, want January to April 2026", won.Start, won.End, won.Chosen)
	}
	if won.Previous != "FY2025/26-Q3" || won.Next == nil || *won.Next != "FY2026/27-Q1" {
		t.Errorf("neighbours = %q and %v, want FY2025/26-Q3 and FY2026/27-Q1", won.Previous, won.Next)
	}
	want := map[string]string{fieldPeriodYear: "FY2026/27", fieldPeriodQuarter: "FY2026/27-Q3", fieldPeriodMonth: "2026-10"}
	for field, label := range want {
		if current[field] != label {
			t.Errorf("current %s = %q, want %q", field, current[field], label)
		}
	}

	year, _, err := resolveInTx(t, e, CompanyRollupPeriod{Field: fieldPeriodYear, Label: "FY2025/26"}, now, zone, 4)
	if err != nil || !year.Start.Equal(time.Date(2025, 4, 1, 0, 0, 0, 0, zone)) || !year.End.Equal(time.Date(2026, 4, 1, 0, 0, 0, 0, zone)) {
		t.Errorf("FY2025/26 = [%s, %s) %v, want April 2025 to April 2026", year.Start, year.End, err)
	}
}

// A label the installation's reports never render, and a period that has not
// begun, are refused naming the parameter rather than read as zero won sales.
func TestARollupPeriodNoReportRendersIsRefused(t *testing.T) {
	e := integration.Setup(t)
	now := time.Date(2026, 10, 30, 9, 0, 0, 0, time.UTC)
	for _, tc := range []struct {
		name   string
		period CompanyRollupPeriod
		start  int
	}{
		{"a calendar quarter under an April start", CompanyRollupPeriod{Field: fieldPeriodQuarter, Label: "2026-Q1"}, 4},
		{"a fiscal year under a January start", CompanyRollupPeriod{Field: fieldPeriodYear, Label: "FY2025/26"}, 1},
		{"a month that does not exist", CompanyRollupPeriod{Field: fieldPeriodMonth, Label: "2026-13"}, 1},
		{"a quarter at the wrong grain", CompanyRollupPeriod{Field: fieldPeriodMonth, Label: "2026-Q1"}, 1},
		{"a month not yet begun", CompanyRollupPeriod{Field: fieldPeriodMonth, Label: "2026-11"}, 1},
		{"a year not yet begun", CompanyRollupPeriod{Field: fieldPeriodYear, Label: "2027"}, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, _, err := resolveInTx(t, e, tc.period, now, time.UTC, tc.start)
			refusal, ok := errors.AsType[*rollupPeriodRefusal](err)
			if !ok || !errors.Is(err, apperrors.ErrInvalidArgument) {
				t.Fatalf("err = %v, want a period refusal", err)
			}
			if field, _, _ := refusal.FieldFault(); field != tc.period.Field {
				t.Errorf("the refusal names %s, want %s", field, tc.period.Field)
			}
		})
	}
}

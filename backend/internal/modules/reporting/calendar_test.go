// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package reporting

import (
	"testing"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
)

func TestScheduleUsesFirstRepeatedTimeAndNextValidTimeAcrossDST(t *testing.T) {
	cases := []struct {
		name, after, want string
		day               int
	}{
		{"spring gap", "2026-03-28T23:00:00Z", "2026-03-29T01:00:00Z", 7},
		{"autumn first occurrence", "2026-10-24T22:00:00Z", "2026-10-25T00:30:00Z", 7},
		{"never second occurrence", "2026-10-25T00:31:00Z", "2026-11-01T01:30:00Z", 7},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			after, err := time.Parse(time.RFC3339, tc.after)
			if err != nil {
				t.Fatal(err)
			}
			got, err := NextDue(crmcontracts.ReportingScheduleInput{Frequency: "weekly", Day: tc.day, LocalTime: "02:30"}, "Europe/Berlin", after)
			if err != nil {
				t.Fatal(err)
			}
			if got.UTC().Format(time.RFC3339) != tc.want {
				t.Fatalf("next run = %s; want %s", got, tc.want)
			}
		})
	}
}

func TestMonthlyScheduleClampsWithoutSkippingShortMonths(t *testing.T) {
	after := time.Date(2027, 2, 1, 0, 0, 0, 0, time.UTC)
	next, err := NextDue(crmcontracts.ReportingScheduleInput{Frequency: "monthly", Day: 31, LocalTime: "09:00"}, "Europe/Berlin", after)
	if err != nil {
		t.Fatal(err)
	}
	if next.UTC().Format(time.RFC3339) != "2027-02-28T08:00:00Z" {
		t.Fatalf("next run = %s", next)
	}
}

func TestWeekAcrossMonthEndUsesTheEndingMonthsActualForTarget(t *testing.T) {
	calendar := Calendar{Timezone: "Europe/Berlin", FiscalStartMonth: 1}
	interval, err := Interval(crmcontracts.ReportingSelection{Period: "last_week"}, calendar, time.Date(2026, 10, 5, 8, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	target, err := TargetWindow(interval, "month", calendar)
	if err != nil {
		t.Fatal(err)
	}
	if interval.StartAt.Day() != 28 || interval.EndAt.Day() != 5 || target.StartAt.Month() != time.October || target.StartAt.Day() != 1 {
		t.Fatalf("wrong interval/target: %+v / %+v", interval, target)
	}
}

func TestThisWeekRunsFromLocalMondayToTheCutoff(t *testing.T) {
	calendar := Calendar{Timezone: "Europe/Berlin", FiscalStartMonth: 1}
	zone, err := time.LoadLocation(calendar.Timezone)
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name, start, target string
		at                  time.Time
	}{
		{"a plain week", "2026-10-12T00:00:00+02:00", "2026-10-01T00:00:00+02:00", time.Date(2026, 10, 14, 9, 30, 0, 0, zone)},
		{"the clocks went back on Sunday", "2026-10-19T00:00:00+02:00", "2026-10-01T00:00:00+02:00", time.Date(2026, 10, 25, 22, 0, 0, 0, zone)},
		{"the clocks go forward on Sunday", "2026-03-23T00:00:00+01:00", "2026-03-01T00:00:00+01:00", time.Date(2026, 3, 29, 23, 0, 0, 0, zone)},
		{"a week that began last month", "2026-10-26T00:00:00+01:00", "2026-11-01T00:00:00+01:00", time.Date(2026, 11, 1, 18, 0, 0, 0, zone)},
	} {
		t.Run(test.name, func(t *testing.T) {
			interval, err := Interval(crmcontracts.ReportingSelection{Period: "this_week"}, calendar, test.at)
			if err != nil {
				t.Fatal(err)
			}
			if got := interval.StartAt.Format(time.RFC3339); got != test.start {
				t.Errorf("the week starts %s, want %s", got, test.start)
			}
			if !interval.EndAt.Equal(test.at) {
				t.Errorf("the week ends %s, want the cutoff %s: a later meeting is not an outcome yet", interval.EndAt, test.at)
			}
			target, err := TargetWindow(interval, "month", calendar)
			if err != nil {
				t.Fatal(err)
			}
			if got := target.StartAt.Format(time.RFC3339); got != test.target {
				t.Errorf("the month target starts %s, want the month holding the cutoff, %s", got, test.target)
			}
		})
	}
}

func TestThisWeekAtMondayMidnightIsAnEmptyWeekInTheCutoffsMonth(t *testing.T) {
	calendar := Calendar{Timezone: "Europe/Berlin", FiscalStartMonth: 1}
	zone, err := time.LoadLocation(calendar.Timezone)
	if err != nil {
		t.Fatal(err)
	}
	monday := time.Date(2027, 3, 1, 0, 0, 0, 0, zone)
	interval, err := Interval(crmcontracts.ReportingSelection{Period: "this_week"}, calendar, monday)
	if err != nil {
		t.Fatalf("the first instant of a week was refused: %v", err)
	}
	if !interval.StartAt.Equal(monday) || !interval.EndAt.Equal(monday) {
		t.Fatalf("interval = %+v, want an empty week at %s", interval, monday)
	}
	target, err := TargetWindow(interval, "month", calendar)
	if err != nil {
		t.Fatal(err)
	}
	if !target.StartAt.Equal(monday) || target.EndAt.Month() != time.April {
		t.Fatalf("target = %+v, want March, the month the empty week starts in", target)
	}
}

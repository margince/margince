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

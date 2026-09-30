// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package reporting

import (
	"errors"
	"testing"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/apperrors"
)

func TestReportingIntervalsRejectInvalidCalendarsAndFutureOrOversizedRanges(t *testing.T) {
	at := time.Date(2026, 9, 29, 12, 0, 0, 0, time.UTC)
	calendar := Calendar{Timezone: "Europe/Berlin", FiscalStartMonth: 4}
	for _, invalidCalendar := range []Calendar{{Timezone: "UTC", FiscalStartMonth: 0}, {Timezone: "unknown", FiscalStartMonth: 1}} {
		if _, err := Interval(crmcontracts.ReportingSelection{Period: "this_month"}, invalidCalendar, at); !errors.Is(err, apperrors.ErrInvalidArgument) {
			t.Fatalf("invalid calendar: %v", err)
		}
		if _, err := TargetWindow(crmcontracts.ReportingWindow{StartAt: at.Add(-time.Hour), EndAt: at}, "month", invalidCalendar); !errors.Is(err, apperrors.ErrInvalidArgument) {
			t.Fatalf("invalid target calendar: %v", err)
		}
	}
	for _, interval := range []*crmcontracts.ReportingWindow{nil, {StartAt: at, EndAt: at.Add(time.Hour)}, {StartAt: at, EndAt: at}, {StartAt: at.AddDate(-2, 0, 0), EndAt: at}} {
		if _, err := Interval(crmcontracts.ReportingSelection{Period: "custom", Interval: interval}, calendar, at); !errors.Is(err, apperrors.ErrInvalidArgument) {
			t.Fatalf("invalid range: %+v %v", interval, err)
		}
	}
	previous, err := Interval(crmcontracts.ReportingSelection{Period: "last_month"}, calendar, at)
	if err != nil {
		t.Fatal(err)
	}
	if previous.StartAt.Format(time.DateOnly) != "2026-08-01" || previous.EndAt.Format(time.DateOnly) != "2026-09-01" {
		t.Fatalf("previous month: %+v", previous)
	}
	quarter, err := Interval(crmcontracts.ReportingSelection{Period: "this_quarter"}, calendar, at)
	if err != nil {
		t.Fatal(err)
	}
	if quarter.StartAt.Format(time.DateOnly) != "2026-07-01" || !quarter.EndAt.Equal(at) {
		t.Fatalf("fiscal quarter: %+v", quarter)
	}
}

func TestReportingSchedulesRejectInvalidCivilTimesAndCadences(t *testing.T) {
	at := time.Date(2026, 9, 29, 12, 0, 0, 0, time.UTC)
	for _, rule := range []crmcontracts.ReportingScheduleInput{
		{Frequency: "weekly", Day: 0, LocalTime: "09:00"},
		{Frequency: "weekly", Day: 8, LocalTime: "09:00"},
		{Frequency: "monthly", Day: 32, LocalTime: "09:00"},
		{Frequency: "daily", Day: 1, LocalTime: "09:00"},
		{Frequency: "weekly", Day: 1, LocalTime: "9:00"},
		{Frequency: "weekly", Day: 1, LocalTime: "24:00"},
		{Frequency: "weekly", Day: 1, LocalTime: "09:60"},
		{Frequency: "weekly", Day: 1, LocalTime: "xx:00"},
	} {
		if _, err := NextDue(rule, "UTC", at); !errors.Is(err, apperrors.ErrInvalidArgument) {
			t.Fatalf("invalid schedule: %+v %v", rule, err)
		}
	}
	if _, err := NextDue(crmcontracts.ReportingScheduleInput{Frequency: "weekly", Day: 1, LocalTime: "09:00"}, "unknown", at); !errors.Is(err, apperrors.ErrInvalidArgument) {
		t.Fatalf("invalid timezone: %v", err)
	}
}

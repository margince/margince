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

func TestCustomReportingRangeStopsAtTheEvaluationCutoff(t *testing.T) {
	calendar := Calendar{Timezone: "Europe/Berlin", FiscalStartMonth: 1}
	at := time.Date(2026, 10, 1, 8, 0, 0, 0, time.UTC)
	for _, tc := range []struct{ name, start, end, want string }{
		{"ten months including future days", "2026-01-01T00:00:00+01:00", "2026-11-01T00:00:00+01:00", "2026-10-01T08:00:00Z"},
		{"through today", "2026-10-01T00:00:00+02:00", "2026-10-02T00:00:00+02:00", "2026-10-01T08:00:00Z"},
		{"completed quarter", "2026-07-01T00:00:00+02:00", "2026-10-01T00:00:00+02:00", "2026-09-30T22:00:00Z"},
		{"twelve calendar months across DST", "2026-01-01T00:00:00+01:00", "2027-01-01T00:00:00+01:00", "2026-10-01T08:00:00Z"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			start, err := time.Parse(time.RFC3339, tc.start)
			if err != nil {
				t.Fatal(err)
			}
			end, err := time.Parse(time.RFC3339, tc.end)
			if err != nil {
				t.Fatal(err)
			}
			selection := crmcontracts.ReportingSelection{Period: "custom", Interval: &crmcontracts.ReportingWindow{StartAt: start, EndAt: end}}
			got, err := Interval(selection, calendar, at)
			if err != nil {
				t.Fatal(err)
			}
			if !got.StartAt.Equal(start) || got.EndAt.UTC().Format(time.RFC3339) != tc.want {
				t.Fatalf("range: %+v", got)
			}
			if !selection.Interval.EndAt.Equal(end) {
				t.Fatal("requested interval changed")
			}
		})
	}
}

func TestCustomReportingRangeValidatesTheRequestedSpanBeforeClamping(t *testing.T) {
	at := time.Date(2026, 10, 1, 8, 0, 0, 0, time.UTC)
	calendar := Calendar{Timezone: "UTC", FiscalStartMonth: 1}
	for _, window := range []crmcontracts.ReportingWindow{
		{StartAt: at.Add(time.Hour), EndAt: at.Add(2 * time.Hour)},
		{StartAt: at.AddDate(0, -1, 0), EndAt: at.AddDate(1, 0, 0)},
		{StartAt: at.AddDate(0, -1, 0), EndAt: at.AddDate(0, 11, 0).Add(time.Nanosecond)},
		{StartAt: at.AddDate(0, -1, 0), EndAt: at.AddDate(0, -2, 0)},
	} {
		if _, err := Interval(crmcontracts.ReportingSelection{Period: "custom", Interval: &window}, calendar, at); !errors.Is(err, apperrors.ErrInvalidArgument) {
			t.Fatalf("invalid range %+v returned %v", window, err)
		}
	}
}

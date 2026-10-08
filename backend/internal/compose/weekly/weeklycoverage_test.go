// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package weekly

import (
	"testing"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
)

func TestAFigureIsJudgedByWhenItsSourceBegan(t *testing.T) {
	t.Parallel()
	start := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	end := start.AddDate(0, 0, 7)
	at := func(d time.Duration) *time.Time { instant := start.Add(d); return &instant }
	cases := []struct {
		name  string
		since *time.Time
		want  crmcontracts.WeeklyFigureCoverageStatus
	}{
		{"a scope that never held one", nil, crmcontracts.WeeklyFigureCoverageStatusNotRecorded},
		{"a first record after the week", at(8 * 24 * time.Hour), crmcontracts.WeeklyFigureCoverageStatusNotRecorded},
		{"a first record at the week's end", at(7 * 24 * time.Hour), crmcontracts.WeeklyFigureCoverageStatusNotRecorded},
		{"a first record inside the week", at(3 * 24 * time.Hour), crmcontracts.WeeklyFigureCoverageStatusPartial},
		{"a first record at the week's first instant", at(0), crmcontracts.WeeklyFigureCoverageStatusRecorded},
		{"a first record before the week", at(-time.Hour), crmcontracts.WeeklyFigureCoverageStatusRecorded},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := figureCoverage(tc.since, start, end, "deal")
			if got.Status != tc.want {
				t.Fatalf("status = %s, want %s", got.Status, tc.want)
			}
			if got.RecordedSince != tc.since {
				t.Errorf("recorded_since = %v, want the source's first record %v", got.RecordedSince, tc.since)
			}
			if (got.Reason != nil) != (tc.want != crmcontracts.WeeklyFigureCoverageStatusRecorded) {
				t.Errorf("reason = %v; only a figure short of the whole week owes one", got.Reason)
			}
		})
	}
}

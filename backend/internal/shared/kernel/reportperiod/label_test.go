// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package reportperiod

import (
	"testing"
	"time"
)

func TestALabelIsReadOnlyInASpellingABucketRenders(t *testing.T) {
	t.Parallel()
	cases := map[string]struct {
		grain Grain
		ok    bool
	}{
		"2026":         {GrainYear, true},
		"FY2025/26":    {GrainYear, true},
		"FY2099/00":    {GrainYear, true},
		"FY2025/99":    {GrainYear, false},
		"FY2025/2026":  {GrainYear, false},
		"2026-Q4":      {GrainQuarter, true},
		"FY2025/26-Q1": {GrainQuarter, true},
		"FY2025/99-Q1": {GrainQuarter, false},
		"2026-Q5":      {GrainQuarter, false},
		"2026-03":      {GrainMonth, true},
		"2026-13":      {GrainMonth, false},
	}
	for label, want := range cases {
		if _, ok := ParseLabel(want.grain, label); ok != want.ok {
			t.Errorf("%s read as %s: ok=%v, want %v", label, want.grain, ok, want.ok)
		}
	}
}

func TestASpanNeedsTheSpellingItsCalendarRenders(t *testing.T) {
	t.Parallel()
	zone, err := time.LoadLocation("Asia/Ho_Chi_Minh")
	if err != nil {
		t.Fatal(err)
	}
	from, to, ok := Span(GrainQuarter, "FY2025/26-Q4", zone, 4)
	if !ok || !from.Equal(time.Date(2026, 1, 1, 0, 0, 0, 0, zone)) || !to.Equal(time.Date(2026, 4, 1, 0, 0, 0, 0, zone)) {
		t.Errorf("FY2025/26-Q4 under April spans %s to %s (%v), want January to April 2026", from, to, ok)
	}
	if _, _, ok := Span(GrainYear, "FY2025/26", zone, 1); ok {
		t.Error("a fiscal spelling named a year under a January start")
	}
	if _, _, ok := Span(GrainYear, "2026", zone, 4); ok {
		t.Error("a calendar spelling named a year under an April start")
	}
}

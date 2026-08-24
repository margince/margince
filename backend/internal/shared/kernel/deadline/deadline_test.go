// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package deadline

import (
	"testing"
	"time"
)

var noon = time.Date(2026, 8, 24, 12, 0, 0, 0, time.UTC)

func at(d time.Duration) *time.Time {
	t := noon.Add(d)
	return &t
}

// The boundary is the whole point of this package: every surface that decides
// lateness agrees here or the same record reads two ways.
func TestPassedIsStrict(t *testing.T) {
	for _, tc := range []struct {
		name string
		due  *time.Time
		want bool
	}{
		{"nothing promised cannot have been missed", nil, false},
		{"due tomorrow", at(24 * time.Hour), false},
		{"due one nanosecond from now", at(time.Nanosecond), false},
		{"due exactly now is DUE, not late", at(0), false},
		{"due one nanosecond ago is late", at(-time.Nanosecond), true},
		{"due yesterday", at(-24 * time.Hour), true},
	} {
		if got := Passed(tc.due, noon); got != tc.want {
			t.Errorf("%s: Passed = %v, want %v", tc.name, got, tc.want)
		}
	}
}

// Zero is a real answer and is not the same as "not late", so callers read the
// bool. A test that only checked the count would pass while the two were
// conflated.
func TestDaysPastSeparatesTheCountFromTheVerdict(t *testing.T) {
	for _, tc := range []struct {
		name     string
		due      *time.Time
		wantDays int
		wantLate bool
	}{
		{"undated", nil, 0, false},
		{"upcoming", at(time.Hour), 0, false},
		{"due exactly now", at(0), 0, false},
		{"late by hours is late by no whole days", at(-5 * time.Hour), 0, true},
		{"late by a day", at(-24 * time.Hour), 1, true},
		{"late by a day and most of another", at(-47 * time.Hour), 1, true},
		{"late by two days", at(-48 * time.Hour), 2, true},
	} {
		days, late := DaysPast(tc.due, noon)
		if days != tc.wantDays || late != tc.wantLate {
			t.Errorf("%s: DaysPast = (%d, %v), want (%d, %v)",
				tc.name, days, late, tc.wantDays, tc.wantLate)
		}
	}
}

// DaysPast must agree with Passed about lateness, or a caller reading one and a
// caller reading the other disagree about the same record.
func TestDaysPastAgreesWithPassed(t *testing.T) {
	for _, offset := range []time.Duration{
		-49 * time.Hour, -24 * time.Hour, -time.Nanosecond, 0, time.Nanosecond, time.Hour,
	} {
		_, late := DaysPast(at(offset), noon)
		if want := Passed(at(offset), noon); late != want {
			t.Errorf("at %v: DaysPast says late=%v, Passed says %v", offset, late, want)
		}
	}
	if _, late := DaysPast(nil, noon); late != Passed(nil, noon) {
		t.Error("the two disagree about an undated promise")
	}
}

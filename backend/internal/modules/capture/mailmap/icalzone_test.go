// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package mailmap

import (
	"testing"
	"time"
)

func TestAZoneOffsetIsReadOnlyInItsTwoWrittenShapes(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		in     string
		want   int
		stated bool
	}{
		{in: "+0100", want: 3600, stated: true},
		{in: "-0500", want: -5 * 3600, stated: true},
		{in: " +053000 ", want: 5*3600 + 30*60, stated: true},
		{in: "-033015", want: -(3*3600 + 30*60 + 15), stated: true},
		{in: "0100", stated: false},
		{in: "+01:00", stated: false},
		{in: "*0100", stated: false},
		{in: "+01a0", stated: false},
		{in: "", stated: false},
	} {
		got, stated := utcOffset(tc.in)
		if got != tc.want || stated != tc.stated {
			t.Errorf("utcOffset(%q) = %d, %v; want %d, %v", tc.in, got, stated, tc.want, tc.stated)
		}
	}
}

func TestAZoneRuleTakesOverOnItsStatedDay(t *testing.T) {
	t.Parallel()
	start := time.Date(1601, 1, 1, 2, 0, 0, 0, time.UTC)
	secondSundayOfMarch := zoneRule{start: start, month: time.March, nth: 2, day: time.Sunday, yearly: true}
	if got, want := secondSundayOfMarch.onsetIn(2026), time.Date(2026, 3, 8, 2, 0, 0, 0, time.UTC); !got.Equal(want) {
		t.Errorf("the second Sunday of March 2026 = %v, want %v", got, want)
	}
	lastSundayOfOctober := zoneRule{start: start, month: time.October, nth: -1, day: time.Sunday, yearly: true}
	if got, want := lastSundayOfOctober.onsetIn(2026), time.Date(2026, 10, 25, 2, 0, 0, 0, time.UTC); !got.Equal(want) {
		t.Errorf("the last Sunday of October 2026 = %v, want %v", got, want)
	}
	oneOff := zoneRule{start: time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)}
	if got := oneOff.onsetIn(2030); !got.Equal(oneOff.start) {
		t.Errorf("a rule with no RRULE takes over at its own start, got %v", got)
	}
}

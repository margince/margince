// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package reportperiod

import (
	"testing"
	"time"
)

func TestComparableRequiresAdjacentCompleteCivilPeriods(t *testing.T) {
	zone, err := time.LoadLocation("Europe/Berlin")
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name         string
		start        time.Time
		months, days int
	}{
		{"month crossing spring DST", time.Date(2026, 3, 1, 0, 0, 0, 0, zone), 1, 0},
		{"quarter crossing spring DST", time.Date(2026, 1, 1, 0, 0, 0, 0, zone), 3, 0},
		{"week crossing spring DST", time.Date(2026, 3, 23, 0, 0, 0, 0, zone), 0, 7},
	} {
		t.Run(test.name, func(t *testing.T) {
			left := Context{Start: test.start, End: test.start.AddDate(0, test.months, test.days), Timezone: zone.String(), Currency: "EUR", Version: "1", Complete: true}
			right := left
			right.Start = left.End
			right.End = right.Start.AddDate(0, test.months, test.days)
			if !Comparable(left, right) {
				t.Fatalf("adjacent civil periods rejected: %+v %+v", left, right)
			}
			right.Start = right.Start.Add(time.Nanosecond)
			if Comparable(left, right) {
				t.Fatal("non-adjacent periods accepted")
			}
		})
	}
	at := time.Date(2026, 1, 1, 0, 0, 0, 0, zone)
	for _, end := range []time.Time{at.AddDate(0, 2, 0), at.AddDate(1, 0, 0), at.AddDate(0, 1, 1)} {
		if kind := CompletedKind(at, end, zone); kind != "" {
			t.Fatalf("irregular interval became %s", kind)
		}
	}
	if kind := CompletedKind(at.Add(time.Nanosecond), at.AddDate(0, 1, 0), zone); kind != "" {
		t.Fatalf("partial day became %s", kind)
	}
	invalid := Context{Start: at, End: at.AddDate(0, 1, 0), Timezone: "unknown", Currency: "EUR", Version: "1", Complete: true}
	if Comparable(invalid, invalid) {
		t.Fatal("invalid timezone accepted")
	}
}

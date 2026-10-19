// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

import (
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/platform/clocktest"
)

func berlinBaseline(day string) *HistoryBaseline {
	date, err := time.Parse(time.DateOnly, day)
	if err != nil {
		panic(err)
	}
	return &HistoryBaseline{Date: date, Timezone: "Europe/Berlin"}
}

func TestABaselineExcludesOnlyLocalDaysStrictlyBeforeIt(t *testing.T) {
	baseline := berlinBaseline("2026-03-29")
	cases := []struct {
		name string
		at   time.Time
		want bool
	}{
		{"the local day before", time.Date(2026, 3, 28, 22, 59, 0, 0, time.UTC), true},
		{"local midnight on the date, still the previous day in UTC", time.Date(2026, 3, 28, 23, 0, 0, 0, time.UTC), false},
		{"the date, after the clocks moved forward", time.Date(2026, 3, 29, 12, 0, 0, 0, time.UTC), false},
		{"a later day", time.Date(2026, 4, 2, 9, 0, 0, 0, time.UTC), false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := baseline.Excludes(tc.at)
			if err != nil {
				t.Fatal(err)
			}
			if got != tc.want {
				t.Errorf("Excludes(%s) = %v, want %v", tc.at, got, tc.want)
			}
		})
	}
}

func TestABaselineReadsTheDateInItsOwnZone(t *testing.T) {
	date, err := time.Parse(time.DateOnly, "2026-10-25")
	if err != nil {
		t.Fatal(err)
	}
	at := time.Date(2026, 10, 25, 2, 0, 0, 0, time.UTC)
	for zone, want := range map[string]bool{"Europe/Berlin": false, "America/New_York": true} {
		got, err := (HistoryBaseline{Date: date, Timezone: zone}).Excludes(at)
		if err != nil {
			t.Fatal(err)
		}
		if got != want {
			t.Errorf("in %s Excludes = %v, want %v", zone, got, want)
		}
	}
}

func TestEveryDeliveringMailboxMustExcludeTheMessage(t *testing.T) {
	early := time.Date(2025, 1, 10, 12, 0, 0, 0, time.UTC)
	cases := []struct {
		name      string
		baselines []*HistoryBaseline
		want      bool
	}{
		{"no proven mailbox", nil, false},
		{"one mailbox with a later baseline", []*HistoryBaseline{berlinBaseline("2026-01-01")}, true},
		{"one mailbox with an earlier baseline", []*HistoryBaseline{berlinBaseline("2024-01-01")}, false},
		{"a second mailbox without a baseline", []*HistoryBaseline{berlinBaseline("2026-01-01"), nil}, false},
		{"two mailboxes that both exclude", []*HistoryBaseline{berlinBaseline("2026-01-01"), berlinBaseline("2025-06-01")}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := allExclude(tc.baselines, early)
			if err != nil {
				t.Fatal(err)
			}
			if got != tc.want {
				t.Errorf("allExclude = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestABaselineInAZoneTheHostCannotLoadFails(t *testing.T) {
	baseline := HistoryBaseline{Date: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), Timezone: "Mars/Olympus"}
	if _, err := baseline.Excludes(clocktest.Now(t)); err == nil {
		t.Error("an unknown zone excluded nothing and reported no error")
	}
}

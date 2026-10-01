// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package reporting

import (
	"testing"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
)

func TestCatchUpRetainsNewestTwelveAndNamesSkippedPeriods(t *testing.T) {
	zone, err := time.LoadLocation("Europe/Berlin")
	if err != nil {
		t.Fatal(err)
	}
	first := time.Date(2026, 1, 5, 9, 0, 0, 0, zone)
	schedule := crmcontracts.ReportingSchedule{NextDueAt: first, Timezone: "Europe/Berlin", Definition: crmcontracts.ReportingScheduleInput{Frequency: "weekly", Day: 1, LocalTime: "09:00"}}
	now := time.Date(2026, 6, 1, 10, 0, 0, 0, zone)
	periods, err := catchUp(schedule, now)
	if err != nil {
		t.Fatal(err)
	}
	if len(periods.Recent) != 12 || periods.Skipped != 10 {
		t.Fatalf("wrong catch-up: %+v", periods)
	}
	if !periods.Recent[11].Equal(time.Date(2026, 6, 1, 9, 0, 0, 0, zone)) || !periods.Next.Equal(time.Date(2026, 6, 8, 9, 0, 0, 0, zone)) {
		t.Fatalf("wrong due boundaries: %+v", periods)
	}
	for _, due := range periods.Recent {
		if due.In(zone).Hour() != 9 || due.Weekday() != time.Monday {
			t.Fatalf("DST shifted local schedule: %s", due)
		}
	}
}

// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package decisionview

import (
	"testing"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
)

func TestTheReviewDayIsTheInstallationDayInAnyZone(t *testing.T) {
	for _, zone := range []string{"America/Los_Angeles", "UTC", "Pacific/Auckland"} {
		loc, err := time.LoadLocation(zone)
		if err != nil {
			t.Fatal(err)
		}
		due := time.Date(2026, 11, 25, 23, 59, 59, 0, loc).UTC()
		for name, intent := range map[string]*crmcontracts.IntentTime{
			"chosen clock":    {WallClock: "2026-11-25T23:59:59", Timezone: zone},
			"stored due only": nil,
		} {
			day := DueDay(intent, &due, loc)
			if day == nil || day.Format(time.DateOnly) != "2026-11-25" {
				t.Errorf("%s in %s read as %v; want 2026-11-25", name, zone, day)
			}
		}
	}
	if DueDay(nil, nil, time.UTC) != nil {
		t.Error("a task with no due date has no review day")
	}
}

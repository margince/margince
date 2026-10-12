// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"testing"
	"time"
)

func TestAScheduleNamesAnIANAZoneTheServerCannotRead(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, time.October, 10, 9, 0, 0, 0, time.UTC)
	at := now.Add(time.Hour)
	for _, zone := range []string{"Local", "local", " Europe/Berlin", "Europe/Berlin ", "UTC+2", "+02:00", "Nowhere/Atlantis"} {
		t.Run(zone, func(t *testing.T) {
			t.Parallel()
			if err := validateSchedule(SendSchedule{At: at, TZ: zone}, now); err == nil {
				t.Fatalf("validateSchedule accepted the zone %q", zone)
			}
		})
	}
	for _, zone := range []string{"Europe/Berlin", "UTC", "America/Argentina/Buenos_Aires"} {
		if err := validateSchedule(SendSchedule{At: at, TZ: zone}, now); err != nil {
			t.Fatalf("validateSchedule refused the zone %q: %v", zone, err)
		}
	}
}

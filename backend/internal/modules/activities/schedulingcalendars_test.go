// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"errors"
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
)

// A null member of blocking_calendars decodes to the empty string; it is
// refused by name instead of stored as a calendar.
func TestABlankBlockingCalendarIsRefusedByName(t *testing.T) {
	t.Parallel()
	profileWith := func(calendars []string) crmcontracts.SchedulingProfile {
		return crmcontracts.SchedulingProfile{
			Provider: "gcal", Title: "Intro call", CalendarId: "primary", DurationMinutes: 30,
			HorizonDays: 14, BlockingCalendars: &calendars,
		}
	}
	for _, calendars := range [][]string{{""}, {"primary", " "}} {
		var refusal *SchedulingArgumentError
		if err := validateSchedulingProfile(profileWith(calendars)); !errors.As(err, &refusal) || refusal.Field != "blocking_calendars" {
			t.Errorf("%q → %v, want a refusal naming blocking_calendars", calendars, err)
		}
	}
	if err := validateSchedulingProfile(profileWith([]string{"primary"})); err != nil {
		t.Errorf("a named calendar was refused: %v", err)
	}
}

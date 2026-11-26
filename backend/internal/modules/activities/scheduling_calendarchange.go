// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"slices"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
)

// bookingCalendarChanged compares what the provider check vouches for. An
// absent blocking list and an empty one block the same nothing.
func bookingCalendarChanged(stored, next crmcontracts.SchedulingProfile) bool {
	return stored.Provider != next.Provider || stored.CalendarId != next.CalendarId ||
		!slices.Equal(blockingCalendars(stored), blockingCalendars(next))
}

func blockingCalendars(p crmcontracts.SchedulingProfile) []string {
	if p.BlockingCalendars == nil {
		return nil
	}
	return *p.BlockingCalendars
}

// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package storekit

import (
	"github.com/margince/margince/backend/internal/shared/kernel/occurredday"
)

// MeetingHeldAsOf are the fragments that read a meeting as its status history
// stood at one moment, rather than as the meeting reads today.
//
// A meeting stays in the week it was held. Reading the row's current
// occurred_at puts one redated afterwards in the week somebody edited it into.
// Two surfaces counting that week then disagree.
type MeetingHeldAsOf struct {
	// Join reads the latest transition at or before the moment asked about.
	Join string
	// Legacy is whether the transition recorded nothing, so the meeting as it
	// reads today is all there is.
	Legacy string
	// Start and Precision place the meeting on its day.
	Start, Precision string
	// UnknownDay is a start whose day cannot be settled from what was
	// recorded, so a caller reads the row under either day.
	UnknownDay string
	// Recorded excludes a row whose history has a later entry than the moment
	// asked about, which is a meeting whose answer is not settled yet.
	Recorded string
}

// MeetingHeldAsOfSQL renders the fragments for the meeting under alias, as its
// history stood at asOf. Both are SQL the caller renders, so a caller binds
// asOf however its own statement binds parameters.
func MeetingHeldAsOfSQL(alias, asOf string) MeetingHeldAsOf {
	return MeetingHeldAsOf{
		Join: `LEFT JOIN LATERAL(SELECT h.* FROM activity_meeting_history h WHERE h.activity_id=` + alias +
			`.id AND h.effective_at<=` + asOf + ` ORDER BY h.effective_at DESC,h.id DESC LIMIT 1) held ON true`,
		Legacy: "(held.id IS NULL OR held.customer_eligible_at_change IS NULL OR held.partial_pre_history)",
		// A transition says whether its start was a time or a date, so a day
		// meeting moved later stays on the day it was held. A row recorded
		// before it said so reads as the meeting's precision only while it is
		// the meeting's time; otherwise as an instant.
		Start: "COALESCE(held.scheduled_start," + alias + ".occurred_at)",
		Precision: "COALESCE(held.scheduled_precision,CASE WHEN held.scheduled_start IS NULL OR held.scheduled_start = " +
			alias + ".occurred_at THEN " + alias + ".occurred_precision ELSE 'instant' END)",
		// Only a noon UTC start can be a date's anchor. On a row with no
		// recorded precision it may be either day.
		UnknownDay: "(held.scheduled_precision IS NULL AND held.scheduled_start IS NOT NULL AND " +
			occurredday.IsAnchorSQL("held.scheduled_start") + ")",
		Recorded: "(held.id IS NOT NULL OR NOT EXISTS(SELECT 1 FROM activity_meeting_history future WHERE future.activity_id=" +
			alias + ".id))",
	}
}

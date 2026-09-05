// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package briefs

// Whether a snooze that waits on the world is over.
//
// Two readers ask this and they must not disagree. The candidate filter asks it
// to decide whether a deal comes back into tomorrow's queue; the mark guard asks
// it to decide whether a rep looking at a stale screen may still act. If the
// filter says an item is back and the guard says it is still set aside, the rep
// sees the row and gets a conflict when they touch it — so the predicate is
// spelled once, here, and both build their query from it.
//
// A `time` snooze is not handled here: it is answered by comparing two
// timestamps, needs no join, and the callers compare it directly.

import (
	"fmt"

	"github.com/margince/margince/backend/internal/modules/activities"
)

// briefSnoozeLiftedSQL renders the predicate as a boolean expression over five
// SQL terms the caller supplies: the deal, the condition, the meeting it may
// name, the instant the item was set down, and the instant to judge against.
//
// Each term is a fragment rather than a fixed column or placeholder because the
// two callers reach the same five values differently — the candidate filter has
// the brief_item row joined and passes column references, the mark guard has
// already read it into Go and passes its own numbered placeholders. Hard-coding
// either shape would compile in one caller and not the other, and hand-typing a
// "$4" into a query whose fourth argument is something else is exactly the
// failure the rule against typing placeholders exists to prevent.
//
// Every term is a compile-time literal at both call sites. Nothing from a
// request body reaches this function.
func briefSnoozeLiftedSQL(deal, condition, ref, setDown, asOf string) string {
	return fmt.Sprintf(`(CASE %s
		-- A reply is anything the counterparty sent us on a conversation
		-- linked to this deal after the rep set it down.
		--
		-- NO CONTENT GATE on the activity, which is deliberate and matches the
		-- dismissal filter this sits beside in briefCandidates: the brief's
		-- queue is scoped by DEAL, and a rep who may read the deal is told
		-- that it moved without being shown what moved it. The waiting-message
		-- lane is scoped by activity instead, so its own predicate does gate
		-- the reply — the two differ because the thing being protected does. Bounded at the
		-- judging instant for the reason the dismissal filter is: a
		-- future-dated inbound has not arrived, and lifting a snooze for it
		-- puts the item back for something still to come.
		WHEN 'reply' THEN EXISTS (
			SELECT 1 FROM activity a
			JOIN activity_link l ON l.activity_id = a.id AND l.deal_id = %s
			WHERE a.archived_at IS NULL
			  AND a.direction = 'inbound'
			  AND a.occurred_at > %s
			  AND a.occurred_at <= %s)
		-- The meeting is over once it has ENDED, which is its start plus its
		-- duration. Lifting at occurred_at would put the work back while the
		-- rep is still in the room, which is the one moment they certainly
		-- cannot act on it. A meeting with no duration is treated as ending
		-- when it starts, because that is all the row says.
		--
		-- A meeting that will not happen counts as over: archived, cancelled or
		-- a no-show. The rep is waiting for something that will never come, and
		-- holding the item forever is the only outcome that loses the work.
		--
		-- kind = 'meeting' is the guard that makes reopen_ref mean what it
		-- says. Without it any activity id serves — an old email whose
		-- occurred_at has long passed lifts the snooze the moment it is set.
		WHEN 'meeting' THEN EXISTS (
			SELECT 1 FROM activity m
			WHERE m.id = %s AND m.kind = '%s'
			  AND (m.archived_at IS NOT NULL
			       OR m.meeting_status IN ('canceled', 'no_show')
			       OR m.occurred_at
			          + make_interval(secs => coalesce(m.duration_seconds, 0)) <= %s))
		ELSE false
	END)`, condition, deal, setDown, asOf, ref, activities.KindMeeting, asOf)
}

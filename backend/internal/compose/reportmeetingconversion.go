// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// Held meeting to accepted opportunity — the SDR funnel's last conversion.
//
// The question is what share of the meetings an SDR got held turned into work an
// account executive took on. Every piece of it already exists: a meeting's own
// history says when it was actually HELD rather than merely booked, and a
// handoff acceptance says an AE took the prospect and names the deal it became.
// This joins them and adds nothing to the schema.
//
// THE ANCHOR IS THE ACCEPTANCE, NEVER A COINCIDENTAL DEAL. The tempting version
// of this report asks "was there a deal at that company afterwards", and it is
// wrong in the direction that flatters: an SDR gets credit for a deal somebody
// else sourced, and the error GROWS WITH ACCOUNT SIZE, because the biggest
// customers generate the most coincidental matches. So the conversion counts a
// meeting only when the prospect it was with was handed on and accepted, which
// is a fact somebody recorded rather than one this report inferred.
//
// ONE MEETING, ONE OPPORTUNITY. The lateral below takes a single handoff per
// meeting, and that is a decision rather than a convenience: without it a
// prospect handed on twice would make one meeting count twice, and a rate whose
// numerator can exceed its denominator is not a rate.

import (
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
)

const (
	// fieldHostUser names the seat that ran the meeting — the SDR whose rate this
	// is. It reads the activity's own host column rather than the handoff's
	// submitter: the question is whose MEETINGS converted, and the contact who
	// held the meeting is not always the contact who later wrote the handoff.
	fieldMeetingHost = "host_user_id"

	// fieldBecameOpportunity is the conversion itself, as a dimension rather
	// than a measure. Grouping by it gives both halves of the rate in one read —
	// the meetings that converted and the meetings that did not — which is what
	// a reader needs to see a rate they can check.
	//
	// It says what it means. `accepted` alone would not: this report joins a
	// handoff whose own status vocabulary already contains `accepted`, so the
	// short name reads as the handoff's field rather than as the meeting's
	// outcome. Every report's default grouping ships in the run_report tool
	// description that every agent listing carries, which is what makes a
	// dimension name a cost as well as a label — but the catalog has room for
	// this one, and buying eight tokens with a name a reader has to decode is
	// the wrong trade.
	fieldBecameOpportunity = "became_opportunity"

	// A held meeting is one the record says was HELD. booked is a plan,
	// no_show and canceled are the two ways a plan fails, and counting any of
	// them here would answer a question about calendars rather than about work.
	whereHeldMeeting = "t.kind = 'meeting' AND t.meeting_status = 'held' AND t.archived_at IS NULL"

	// joinAcceptedHandoff is the anchor.
	//
	// LATERAL with LIMIT 1 rather than a plain join, because the join CAN
	// multiply: one prospect may be handed on more than once — recycled and
	// resubmitted is an ordinary path — and a meeting counted once per handoff
	// would inflate the numerator above the denominator. The lateral takes the
	// EARLIEST acceptance, so a prospect accepted, lost, and handed on again a
	// year later credits the meeting that preceded the first acceptance rather
	// than whichever one happens to sort last.
	//
	// LEFT, because a meeting that converted into nothing is the other half of
	// the rate. An inner join would silently answer "what share of converted
	// meetings converted", which is 100% and tells nobody anything.
	//
	// The join reaches the prospect through activity_link, which carries both a
	// lead_id and a contact_id — the same two subjects a handoff names, so the
	// two shapes meet without a translation step in between.
	// The select list carries h.id ALONE. The acceptance's deal_id is the
	// tempting thing to add — it names what the meeting became — but this
	// report never reads it, and a deal reference handed back from a join is a
	// row-scoped record served without its scope: the meeting is discoverable
	// to its host, the deal need not be, and
	// TestEveryComposeReadOfARecordReferenceAppliesItsRowScope refuses the pair.
	// A report that wants the deal has to walk the deal's own scope to get it.
	joinAcceptedHandoff = `LEFT JOIN LATERAL (
		SELECT h.id, h.decided_at
		  FROM sdr_handoff h
		  JOIN activity_link al ON al.activity_id = t.id
		 WHERE h.status = 'accepted'
		   AND (h.lead_id = al.lead_id OR h.contact_id = al.contact_id)
		   AND h.decided_at >= t.occurred_at
		 ORDER BY h.decided_at
		 LIMIT 1
	) hoff ON true`

	// The acceptance must come AFTER the meeting, which the join's own predicate
	// says. A prospect accepted in March and met in June was not converted by
	// that June meeting, and without the ordering the report would credit a
	// meeting for an opportunity that already existed when it happened.
	colBecameOpportunity = "(hoff.id IS NOT NULL)"
)

// meetingConversionSpec is the held-meeting-to-accepted-opportunity key.
//
// The grain is ONE ROW PER HELD MEETING, so a meeting counts once whether the
// prospect was handed on once or three times. The default grouping is the
// conversion flag, because the first thing anybody asks of this report is the
// rate — and a rate read off two rows is one a reader can check, where a single
// computed percentage is one they have to trust.
func meetingConversionSpec() reportSpec {
	return reportSpec{
		entity: datasource.EntityActivity,
		table:  tableActivity,
		joins:  []string{joinAcceptedHandoff},
		// Row scope for an activity is real per-row discoverability rather than
		// the identity-table TRUE a deal's is, so this walks it the way
		// activities-by-kind does. The population default's hardcoded owner_id
		// clause would render invalid SQL against a table with no owner_id at
		// all — a 500 rather than a wrong answer, which is why the walk is named
		// rather than left to the default.
		population:   measureEveryReadableRow,
		activityWalk: true,
		baseWhere:    whereHeldMeeting,
		basePlain: "meetings the record says were HELD (a booked one is a plan, and a no-show " +
			"or a cancellation is a plan that failed)",
		dimensions: map[string]string{
			fieldMeetingHost:       colHostUserID,
			fieldBecameOpportunity: colBecameOpportunity,
		},
		measures: map[string]string{},
		filters: map[string]string{
			fieldMeetingHost:       colHostUserID,
			fieldBecameOpportunity: colBecameOpportunity,
		},
		defaultBy: []string{fieldBecameOpportunity},
		// The conversion is a fact about a HANDOFF, so it takes the handoff's
		// object grant. sdr_handoff gates on `lead` (contacts/sdrhandoff.go), and
		// a seat holding activity.read alone can see the meeting while having no
		// business reading what an AE decided about the prospect.
		//
		// BOTH the dimension and the filter are named. They are separate entries
		// in this spec and separate disclosures: the dimension hands the fact
		// over in bulk — it is this report's DEFAULT grouping, so an unfiltered
		// run would serve it having named nothing — and the filter asks the same
		// question one meeting at a time, which is an oracle rather than a
		// report. A grant covering only the dimension leaves that arm open.
		grants: map[string]string{
			fieldBecameOpportunity: tableLead,
		},
		// The count alone, deliberately. A distinct-deal figure beside it would
		// answer a real question — two meetings handed on into ONE deal are two
		// conversions and one opportunity — and this engine has no count_distinct
		// to answer it with. Adding one for a single report would be a new
		// aggregate in a closed vocabulary every other spec shares; the question
		// is worth its own change rather than a widening smuggled in here.
		defaultAggs: []reportAggregate{
			{Fn: aggFnCount, As: "meetings"},
		},
		// NO notes line, and the omission is a decision this file has to record
		// because a later author will reach for one.
		//
		// Every report's notes ship in the run_report description that every
		// agent listing carries, so a note is paid for by every agent run
		// forever. What one would have said here is in this file's header
		// instead, where a reader gets the whole argument and an agent pays
		// nothing for it. That trade holds whatever the catalog's current
		// headroom is; it is not a rationing decision.
	}
}

// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package attention

// The meeting concern: how a booked meeting becomes an item, and how that item
// is ranked. Both halves read the same two facts — when it starts, and whether
// anybody has written anything down for it — so they live together rather than
// one file apart, where a change to the hint could stop the ranking reading it.

import (
	"time"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// meetingItem renders one appointment still ahead today.
//
// The subject IS the sentence a rep recognises — somebody wrote it when the
// meeting was booked — so it travels as the title, and the start time as
// `due_at` because that is what a reader is racing. No `overdue` flag: a
// meeting that has not started cannot be late, and the lane only carries the
// ones still ahead.
//
// It offers no `open`, and for a reason that still holds: this card's subject is
// the ACTIVITY, which is a timeline entry rather than a record with a page of
// its own, so the verb would advertise a destination that does not exist. The
// two cards whose subject IS a record — the quiet deal and the promise — send it.
//
// It DOES offer the brief, where the meeting names somebody. The brief is not a
// page either, which is why this read as unreachable for so long: it opens as
// `?prep=<activity>` on a CONTACT's record, so the move needs both ids and the
// row already carried only one. The contact rides in the move's arguments rather
// than as the row's subject, because the subject names what the row is ABOUT
// and this row is about the meeting.
//
// The brief itself is still its own surface with its own cited sections. This
// names the way in; a queue that tried to summarise it here would be a second
// answer to "prepare me for this".
//
// It offers no OUTCOME, and that is a fact about THIS lane rather than about
// the product: every row here is a meeting still ahead, and a meeting that has
// not happened has no result to record. The rows that do are on the lane
// beside this one — see meetingAwaitingOutcomeItem below.
func meetingItem(meeting Meeting) crmcontracts.AttentionItem {
	subject := meeting.Subject
	starts := meeting.StartsAt
	item := crmcontracts.AttentionItem{
		Id:      meeting.ID.String(),
		Source:  crmcontracts.AttentionItemSource("meeting"),
		Title:   &subject,
		Subject: subjectOf("activity", meeting.ID),
		DueAt:   &starts,
		Actions: []crmcontracts.AttentionItemActions{},
	}
	// Only where a contact is named. A meeting with no attendee this reader may
	// see has no page to read the brief on, so the field stays absent and the
	// classifier below offers no way in — rather than one that opens nothing.
	if !meeting.ContactID.IsZero() {
		with := openapi_types.UUID(meeting.ContactID)
		item.WithContact = &with
	}
	// `kind` is the producer's own sub-type, for the icon and the label and
	// never for authority — which is exactly what "nobody has written anything
	// down for this yet" is. Carried here rather than as a field of its own so
	// the contract gains no vocabulary for a display hint.
	//
	// Set only when the answer is KNOWN. A meeting whose content this reader may
	// not read arrives with an empty body for a reason that is not preparation,
	// and `meetingPrep` refuses to guess; an absent kind is that refusal
	// reaching the page, where it draws nothing.
	if meeting.PrepKnown && meeting.NeedsPrep {
		kind := meetingKindUnprepared
		item.Kind = &kind
	}
	// Whose calendar it came off, where one claims it. Absent for a meeting
	// booked in the app or captured before the host was recorded — the row then
	// names nobody rather than guessing at an owner.
	if !meeting.HostUserID.IsZero() {
		host := openapi_types.UUID(meeting.HostUserID)
		item.HostUserId = &host
	}
	return item
}

// The two meeting lanes' source words, as the wire spells them.
//
// Read by scope.go, which has to know that both lanes answer the ownership
// question in their own query. A second spelling would be silent in the
// direction that matters: the comparison would simply never match, the scope
// filter would re-judge rows it must not, and an invited colleague's meetings
// would vanish from their own page with nothing to say why.
//
// Held by: TestTheMeetingSourceWordsAreNotRetyped and
// TestTheMeetingSourceConstantsMatchTheWire
// (backend/internal/compose/attention/meetingsourcespelling_test.go)
const (
	sourceMeeting        = "meeting"
	sourceMeetingOutcome = "meeting_outcome"
)

// meetingKindUnprepared marks a meeting with nothing written down for it.
//
// One spelling, written by meetingItem and read by classifyMeeting. A second
// would be silent: the writer would stamp a word the reader never matches, and
// the reason would simply stop appearing on rows that had earned it.
//
// Held by: TestTheUnpreparedMarkHasOneSpelling
// (backend/internal/compose/attention/meetingpreplane_test.go)
const meetingKindUnprepared = "unprepared"

// classifyMeeting: a meeting starting within the horizon is the most urgent
// thing on the page, because it happens whether or not the reader acts.
func classifyMeeting(item crmcontracts.AttentionItem, asOf time.Time) ranked {
	level := levelAgreed
	reasons := []crmcontracts.WorklistReason{}
	if item.DueAt != nil && item.DueAt.Sub(asOf) <= meetingHorizon {
		level = levelWaiting
		reasons = append(reasons, reason("meeting_soon", nil))
	}
	// Nothing written down for it, said only where the lane could actually tell
	// — an absent kind means the reader may not read the row's content, not
	// that the meeting is ready. Silence beats a warning nobody can act on.
	if item.Kind != nil && *item.Kind == meetingKindUnprepared {
		reasons = append(reasons, reason("meeting_unprepared", nil))
	}
	row := base(item, level, "meetings", "meeting_unprepared")
	// The way into the brief, where the row named a contact to read it on. Both
	// ids travel because neither names it alone: the activity says WHICH
	// meeting, the contact says WHOSE page it opens on.
	// The meeting id comes off the SUBJECT rather than being parsed back out of
	// item.Id: the subject already holds it as a uuid, and re-parsing the
	// string form would introduce a failure case where there is none.
	if item.WithContact != nil && item.Subject != nil {
		meetingID := item.Subject.Id
		row.Move = &crmcontracts.WorklistMove{
			Action:     crmcontracts.WorklistMoveActionOpenMeetingBrief,
			ActivityId: &meetingID,
			Arguments:  &map[string]any{"contact_id": item.WithContact.String()},
		}
	}
	// A meeting's start time IS a deadline the reader is racing, so it counts
	// as work due — unlike a proposal's expiry, which merely lapses.
	stampDeadline(&row, item.DueAt, asOf)
	row.Because = reasons
	return ranked{
		item:       row,
		deadlineAt: deadlineOf(item.DueAt),
		occurredAt: occurredOf(item, asOf),
		// The HOST, and never the reader. A team-scoped reader receives their
		// team's meetings, so naming the reader would make every colleague's
		// appointment look like their own — which is what this row used to do,
		// by naming nobody at all.
		//
		// ownerFrom answers unassigned for a meeting no calendar claims, which is
		// the honest answer for one booked in the app.
		ownerRef: ownerFrom(hostOf(item)),
	}
}

// hostOf reads the host id back off the wire item, which is where the lane put
// it. Zero when the row names no host.
func hostOf(item crmcontracts.AttentionItem) ids.UUID {
	if item.HostUserId == nil {
		return ids.UUID{}
	}
	return ids.UUID(*item.HostUserId)
}

// meetingAwaitingOutcomeItem draws one meeting that happened and owes an answer.
//
// The counterpart of meetingItem, and deliberately thinner. That row is about
// preparing, so it carries a contact to read the brief on and a prep tri-state;
// this one is about closing off, which needs the meeting and nothing else.
//
// `OccurredAt` and no `DueAt`. The lane above races a start time, so its rows
// carry a deadline; a meeting that already began cannot be late, and stamping
// one would put an overdue mark on a row whose whole point is that the meeting
// is over.
//
// It offers `decide`, the same verb an approval carries and for the same
// reason: the answer is given ON the row rather than somewhere else. What the
// row asks is how the meeting went, and the three answers — held, no-show,
// cancelled — are the meeting_status vocabulary the activity has always
// contracted. PATCH /activities/{id} has accepted them since the field existed;
// what was missing was anywhere to say so at the one moment a human knows.
//
// Not `open`: this row's subject is the ACTIVITY, a timeline entry with no page
// of its own, so a verb promising to open it would advertise a destination that
// does not exist — the same reason meetingItem above offers none.
func meetingAwaitingOutcomeItem(meeting MeetingAwaitingOutcome) crmcontracts.AttentionItem {
	subject := meeting.Subject
	started := meeting.StartedAt
	item := crmcontracts.AttentionItem{
		Id:         meeting.ID.String(),
		Source:     crmcontracts.AttentionItemSource("meeting_outcome"),
		Title:      &subject,
		OccurredAt: &started,
		Subject:    subjectOf("activity", meeting.ID),
		Actions: []crmcontracts.AttentionItemActions{
			crmcontracts.AttentionItemActionsDecide,
		},
		Version: meeting.Version,
	}
	// The same owner the forward lane names, for the same reason: a row that
	// says whose meeting it was is one a manager can read a team's day from.
	if !meeting.HostUserID.IsZero() {
		host := openapi_types.UUID(meeting.HostUserID)
		item.HostUserId = &host
	}
	return item
}

// classifyUnansweredMeeting places a meeting that happened and owes an answer.
//
// levelAgreed, the same rung the lane beside it starts on and never above it: a
// meeting nobody has closed off is real admin, and putting it over a customer
// waiting for a reply would rank tidying above answering.
//
// It carries no deadline. stampDeadline is what marks a row overdue, and this
// row has no due moment to give it — the meeting already happened, so being
// late is not a state it can be in.
func classifyUnansweredMeeting(item crmcontracts.AttentionItem, asOf time.Time) ranked {
	// data_drifts, from the queue's own closed vocabulary of consequences: what
	// an unanswered meeting costs is that the record stops matching what
	// happened, so every count over held meetings reads short. It is not a
	// second spelling of the reason beside it — the reason says what is true,
	// the consequence says what it costs.
	row := base(item, levelAgreed, "meetings", "data_drifts")
	row.Because = []crmcontracts.WorklistReason{reason("outcome_unrecorded", nil)}
	return ranked{
		item:       row,
		occurredAt: occurredOf(item, asOf),
		// The host, exactly as classifyMeeting names one — the two lanes are the
		// same meetings asked about from either side of their start time, and an
		// owner on one but not the other would file the same appointment under
		// two different contacts as the day goes past it.
		ownerRef: ownerFrom(hostOf(item)),
	}
}

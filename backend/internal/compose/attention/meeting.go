// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package attention

// The meeting concern: how a booked meeting becomes an item, and how that item
// is ranked. Both halves read the same two facts — when it starts, and whether
// anybody has written anything down for it — so they live together rather than
// one file apart, where a change to the hint could stop the ranking reading it.

import (
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
)

// meetingItem renders one appointment still ahead today.
//
// The subject IS the sentence a rep recognises — somebody wrote it when the
// meeting was booked — so it travels as the title, and the start time as
// `due_at` because that is what a reader is racing. No `overdue` flag: a
// meeting that has not started cannot be late, and the lane only carries the
// ones still ahead.
//
// It offers NO verb. The pre-meeting brief is its own surface with its own eight
// cited sections, and a queue that tried to summarise it here would be a second
// answer to "prepare me for this". `open` is withheld for a narrower reason than
// it once was: this card's subject is the ACTIVITY, which is a timeline entry
// rather than a record with a page of its own, so the verb would advertise a
// destination that does not exist. The two cards whose subject IS a record —
// the quiet deal and the promise — now send it.
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
	return item
}

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
	// A meeting's start time IS a deadline the reader is racing, so it counts
	// as work due — unlike a proposal's expiry, which merely lapses.
	stampDeadline(&row, item.DueAt, asOf)
	row.Because = reasons
	return ranked{
		item:       row,
		deadlineAt: deadlineOf(item.DueAt),
		occurredAt: occurredOf(item, asOf),
		// NOT the reader's, and the obvious claim here is false. The lane lists
		// meeting activities under the caller's ROW SCOPE — no owner or attendee
		// predicate — so a team-scoped reader receives their team's meetings and
		// naming the reader would make every one of them look like the reader's
		// own appointment.
		//
		// The activity carries no owner this lane reads, so nobody is named. A
		// meeting's real owner is its organiser, which arrives with the attendee
		// read that does not exist yet.
		ownerRef: unassigned(),
	}
}

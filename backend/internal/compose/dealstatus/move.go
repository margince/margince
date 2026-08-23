// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package dealstatus

// Which move the card offers, decided from records.
//
// The VERB is never the model's to choose. A reader clicking "Draft the reply"
// must reach the mail these rules picked; a model that named a different one
// would send them somewhere the button cannot go. The lane writes why the move
// is right, and these rules decide what clicking it does.
//
// The rules are ordered by how much the record already tells us. A booked
// meeting outranks an unanswered mail because the meeting has a date; an
// unanswered mail outranks a bare next step because somebody is waiting.
//
// An open task no longer ENDS the reasoning. It used to: the card read the
// task's title back to the reader and said it had nothing to add, which is a
// card telling you what you already know and then declining to help. An open
// task is now evidence like any other, and the deal still gets a move.

import (
	"fmt"
	"time"

	crmcontracts "github.com/gradionhq/margince/backend/internal/contracts"
)

// The verbs the client performs on click. Unchanged from the card this
// replaces, so a client that already performs them needs no new code.
const (
	ActionDraftEmail       = "draft_email"
	ActionCreateTask       = "create_task"
	ActionOpenMeetingBrief = "open_meeting_brief"
	ActionNone             = "none"
)

// meetingHorizon bounds how far ahead a booked meeting still counts as the
// next move. Past it, the meeting is a plan rather than a thing to prepare.
const meetingHorizon = 14 * 24 * time.Hour

// decideMove picks the move from the records. A closed deal gets none: there
// is nothing to advance, and inventing a step would be the card talking for
// the sake of filling its own layout.
func decideMove(f facts) crmcontracts.DealStatusCardMove {
	if f.deal.Status != crmcontracts.DealStatusOpen {
		return crmcontracts.DealStatusCardMove{
			Action:   ActionNone,
			Reason:   fmt.Sprintf("This deal is %s — there is no next step to take.", f.deal.Status),
			Evidence: []crmcontracts.DealNextBestActionEvidence{},
		}
	}
	if meeting, ok := upcomingMeeting(f); ok {
		return move(ActionOpenMeetingBrief,
			fmt.Sprintf("A meeting is booked %s — read the brief before it.", until(f.now, meeting.OccurredAt)),
			map[string]any{"activity_id": meeting.Id},
			evidenceOf(meeting, "Booked: "+subjectOf(meeting)))
	}
	if inbound, ok := unansweredInbound(f); ok {
		return move(ActionDraftEmail,
			fmt.Sprintf("They wrote %s and nobody has answered — draft the reply.", since(f.now, inbound.OccurredAt)),
			map[string]any{"activity_id": inbound.Id},
			evidenceOf(inbound, "Unanswered: "+subjectOf(inbound)))
	}
	return move(ActionCreateTask, nextStepReason(f),
		map[string]any{
			"subject": "Agree the next step on " + f.deal.Name,
			"links":   []map[string]any{{"entity_type": "deal", "entity_id": f.deal.Id}},
			"source":  "ui",
		},
		lastContactEvidence(f)...)
}

func move(action, reason string, args map[string]any, evidence ...crmcontracts.DealNextBestActionEvidence) crmcontracts.DealStatusCardMove {
	out := crmcontracts.DealStatusCardMove{Action: action, Reason: reason, Arguments: &args}
	out.Evidence = append([]crmcontracts.DealNextBestActionEvidence{}, evidence...)
	return out
}

// upcomingMeeting is the soonest booked meeting inside the horizon. A meeting
// with no status is booked — the predicate person360 and org360's next-meeting
// reads spell — so the card and the record pages agree about which is next.
func upcomingMeeting(f facts) (crmcontracts.Activity, bool) {
	var best crmcontracts.Activity
	found := false
	for _, a := range f.timeline {
		if a.Kind != crmcontracts.ActivityKindMeeting || withheld(a) {
			continue
		}
		if a.MeetingStatus != nil && *a.MeetingStatus != crmcontracts.ActivityMeetingStatusBooked {
			continue
		}
		if a.OccurredAt.Before(f.now) || a.OccurredAt.After(f.now.Add(meetingHorizon)) {
			continue
		}
		if !found || a.OccurredAt.Before(best.OccurredAt) {
			best, found = a, true
		}
	}
	return best, found
}

// unansweredInbound is the latest inbound MAIL with no outbound mail after it.
// Mail only: the verb this arm names drafts a threaded reply, which the draft
// path composes only for an inbound email — an inbound call has no thread to
// answer on, and falls through to the next rule.
func unansweredInbound(f facts) (crmcontracts.Activity, bool) {
	for _, a := range f.timeline {
		if a.Kind != crmcontracts.ActivityKindEmail || a.Direction == nil || a.OccurredAt.After(f.now) || withheld(a) {
			continue
		}
		if *a.Direction == crmcontracts.ActivityDirectionInbound {
			return a, true
		}
		return crmcontracts.Activity{}, false
	}
	return crmcontracts.Activity{}, false
}

// lastContact is the newest row that has already happened. The timeline's
// first rows can be scheduled meetings with future times, which are plans and
// not contact.
func lastContact(f facts) (crmcontracts.Activity, bool) {
	for _, a := range f.timeline {
		if !a.OccurredAt.After(f.now) {
			return a, true
		}
	}
	return crmcontracts.Activity{}, false
}

func nextStepReason(f facts) string {
	last, ok := lastContact(f)
	if !ok {
		return "Nothing has been logged on this deal yet — agree the next step."
	}
	return fmt.Sprintf("The last contact was %s and nothing is booked — agree the next step.",
		since(f.now, last.OccurredAt))
}

func lastContactEvidence(f facts) []crmcontracts.DealNextBestActionEvidence {
	last, ok := lastContact(f)
	if !ok {
		return nil
	}
	return []crmcontracts.DealNextBestActionEvidence{evidenceOf(last, "Last contact: "+subjectOf(last))}
}

func evidenceOf(a crmcontracts.Activity, text string) crmcontracts.DealNextBestActionEvidence {
	id := a.Id
	at := a.OccurredAt
	return crmcontracts.DealNextBestActionEvidence{Text: text, ActivityId: &id, OccurredAt: &at}
}

// subjectOf names a row for a reader. A withheld row is named by its kind:
// the reader may know contact happened without reading what was said.
func subjectOf(a crmcontracts.Activity) string {
	if withheld(a) || a.Subject == nil || *a.Subject == "" {
		return string(a.Kind)
	}
	return *a.Subject
}

func since(now, then time.Time) string {
	return spell(calendarDaysBetween(then, now))
}

func until(now, then time.Time) string {
	days := calendarDaysBetween(now, then)
	switch days {
	case 0:
		return "today"
	case 1:
		return "tomorrow"
	default:
		return fmt.Sprintf("in %d days", days)
	}
}

// calendarDaysBetween counts whole days between two moments by the CALENDAR,
// not by elapsed hours.
//
// It has to, because the cache is keyed on the UTC day. Counting elapsed hours
// would flip "today" to "yesterday" 24 hours after the contact — mid-afternoon,
// say — while the fingerprint waits for midnight, and the card would spend the
// gap saying the wrong thing with nothing to notice. Counting the same way the
// key does means the wording can only change when the key does.
func calendarDaysBetween(from, to time.Time) int {
	fromDay := from.UTC().Truncate(24 * time.Hour)
	toDay := to.UTC().Truncate(24 * time.Hour)
	return int(toDay.Sub(fromDay).Hours() / 24)
}

func spell(days int) string {
	switch {
	case days <= 0:
		return "today"
	case days == 1:
		return "yesterday"
	default:
		return fmt.Sprintf("%d days ago", days)
	}
}

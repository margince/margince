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

import (
	"fmt"
	"time"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/dealrole"
	"github.com/margince/margince/backend/internal/shared/kernel/elapsed"
)

// The verbs the client performs on click. Unchanged from the card this
// replaces, so a client that already performs them needs no new code.
const (
	ActionDraftEmail       = "draft_email"
	ActionCreateTask       = "create_task"
	ActionOpenTask         = "open_task"
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
	if opening, ok := firstOutreach(f); ok {
		return opening
	}
	// Existing work is the next step until somebody completes it. Creating
	// another generic follow-up here duplicates the task the card just read.
	if len(f.openTasks) > 0 {
		task := f.openTasks[0]
		activityID := openapi_types.UUID(task.ID)
		return move(ActionOpenTask, "Complete the existing task: "+task.Subject, map[string]any{"activity_id": activityID},
			crmcontracts.DealNextBestActionEvidence{ActivityId: &activityID, Text: task.Subject})
	}
	return move(ActionCreateTask, nextStepReason(f),
		map[string]any{
			"subject": "Agree the next step on " + f.deal.Name,
			"links":   []map[string]any{{"entity_type": "deal", "entity_id": f.deal.Id}},
			"source":  "ui",
		},
		lastContactEvidence(f)...)
}

// The stakeholder roles this file reasons about. They are the wire values
// compose/network serves on a coverage seat, not this package's own
// vocabulary — spelling one inline is how it ends up a typo that never
// matches, which fails silently because a role that matches nothing simply
// produces no advice.
//
// Held by: TestTheRoleVocabularyIsSpelledOnce (rolevocabulary_test.go)
const (
	roleChampion      = dealrole.Champion
	roleEconomicBuyer = dealrole.EconomicBuyer
	roleDecisionMaker = "decision_maker"
	roleInfluencer    = "influencer"
)

// openingRoles is who to write to first, best answer first. A champion will
// carry the conversation internally; an economic buyer can decide; a
// decision-maker or an influencer is a way in. A blocker is deliberately
// absent — opening a deal by writing to the person most likely to refuse it is
// not a first move, and suggesting it would be worse than saying nothing.
var openingRoles = []string{roleChampion, roleEconomicBuyer, roleDecisionMaker, roleInfluencer}

// firstOutreach is the move on a deal nobody has contacted yet.
//
// Without it such a deal was told "Nothing has been logged on this deal yet —
// agree the next step", which restates the empty timeline the reader is
// looking at and names no person, no role and no verb they could not have
// worked out themselves. A deal with named seats and no contact has exactly
// one obvious next move, and the records already say who it is with.
//
// It refuses to guess in three cases, and each one returns no move rather than
// a vague one: a deal that has been contacted (the later rules own it), a deal
// with no seats at all, and a deal whose only seats hold roles this cannot
// order. The last is the one worth stating — an unrecognized role is not a
// reason to write to somebody, and picking arbitrarily would put a stranger's
// name in an instruction.
func firstOutreach(f facts) (crmcontracts.DealStatusCardMove, bool) {
	if _, contacted := lastContact(f); contacted {
		return crmcontracts.DealStatusCardMove{}, false
	}
	for _, role := range openingRoles {
		seat, ok := namedRole(f.seats, role)
		if !ok {
			continue
		}
		return move(ActionDraftEmail, openingReason(seat, len(f.seats)), nil), true
	}
	return crmcontracts.DealStatusCardMove{}, false
}

// openingReason says who to open with and why they are the one.
//
// It carries the seat COUNT because that is the fact a reader checks the
// advice against: "four people are named and none has been contacted" is
// checkable against the page, where "reach out" is not.
func openingReason(seat Seat, seats int) string {
	who := roleWord(seat.Role)
	if seat.Name != "" {
		who = seat.Name + ", the " + roleWord(seat.Role)
	}
	if seats == 1 {
		return fmt.Sprintf("Nobody has been contacted yet. Open with %s.", who)
	}
	return fmt.Sprintf("%d people are named on this deal and none has been contacted. Open with %s.", seats, who)
}

// roleWords is the wire value on the left, the words a sentence uses on the
// right. Two of them read the same and are still two different things: the key
// is what compose/network stores, the value is English prose, and a rename on
// either side must not silently move the other.
var roleWords = map[string]string{
	roleChampion:      "champion",
	roleEconomicBuyer: "economic buyer",
	roleDecisionMaker: "decision-maker",
	roleInfluencer:    "influencer",
}

// roleWord is a stakeholder role as a sentence says it. An unknown role is
// returned as it is stored rather than dropped: the card would otherwise write
// "Open with Maria Schmidt, the ." for a role added after this build.
func roleWord(role string) string {
	if word, known := roleWords[role]; known {
		return word
	}
	return role
}

func move(action, reason string, args map[string]any, evidence ...crmcontracts.DealNextBestActionEvidence) crmcontracts.DealStatusCardMove {
	// An empty object for a verb that takes no operand, never a nil map. The
	// contract types `arguments` as an object, and a nil map behind a non-nil
	// pointer serializes as `"arguments": null` — off-contract, and a client
	// reading it as an object gets null where it indexes.
	if args == nil {
		args = map[string]any{}
	}
	out := crmcontracts.DealStatusCardMove{Action: action, Reason: reason, Arguments: &args}
	if action == ActionNone {
		out.Arguments = nil
	}
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
//
// It answers for the card's move AND for reply_to, so the button and the email
// box in the deal's margin cannot disagree about whether somebody is waiting.
//
// Held by: TestTheMoveAndReplyToNameTheSameMail (move_test.go)
func unansweredInbound(f facts) (crmcontracts.Activity, bool) {
	for _, a := range f.timeline {
		if a.Kind != crmcontracts.ActivityKindEmail || a.Direction == nil || a.OccurredAt.After(f.now) {
			continue
		}
		// A withheld row still ANSWERS. Only its words are hidden from this
		// reader, and skipping it would walk past the reply to the inbound
		// behind it and report a mail as unanswered after somebody answered
		// it. So an outbound ends the scan whether or not it can be read, and
		// only a readable inbound is offered — there is no drafting a reply to
		// a message whose text this reader may not see.
		if *a.Direction != crmcontracts.ActivityDirectionInbound {
			return crmcontracts.Activity{}, false
		}
		if withheld(a) {
			return crmcontracts.Activity{}, false
		}
		return a, true
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

// evidenceOf points the card at one record. A ZERO timestamp answers nil
// rather than a date.
//
// occurred_at is nullable on the wire because some records genuinely have no
// moment: an open task with no due date is the case, and taking the address of
// its zero value renders `0001-01-01T00:00:00Z` — a date no reader can act on
// and none of them chose. "There is no date" is what nil says, and it is the
// truth about that row.
func evidenceOf(a crmcontracts.Activity, text string) crmcontracts.DealNextBestActionEvidence {
	id := a.Id
	out := crmcontracts.DealNextBestActionEvidence{Text: text, ActivityId: &id}
	if !a.OccurredAt.IsZero() {
		at := a.OccurredAt
		out.OccurredAt = &at
	}
	return out
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

// calendarDaysBetween counts whole days between two moments by the CALENDAR.
//
// The arithmetic is shared/kernel/elapsed's: the coverage chips beside this
// card count the same silence, and the two used to disagree on screen because
// each carried its own spelling. See that package for why the calendar and not
// the clock.
func calendarDaysBetween(from, to time.Time) int {
	return elapsed.Days(from, to)
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

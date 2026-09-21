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
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/shared/kernel/dealrole"
	"github.com/margince/margince/backend/internal/shared/kernel/elapsed"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/nextstep"
	"github.com/margince/margince/backend/internal/shared/kernel/relstrength"
)

// The verbs the client performs on click. Unchanged from the card this
// replaces, so a client that already performs them needs no new code.
// The argument keys the client reads off a move, and the link vocabulary its
// task body uses. They are named rather than typed at each site because a typo
// in one copy is a button that carries no operand and says nothing about why.
const (
	argActivityID = "activity_id"
	argEntityType = "entity_type"
	argEntityID   = "entity_id"
	linkDeal      = "deal"
	linkContact   = "contact"
)

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

// decideMove keeps operational obligations separate from sales progression.
// Closing a deal ends pipeline advice, not meetings, tasks or customer requests.
func decideMove(f facts) crmcontracts.DealStatusCardMove {
	if meeting, ok := upcomingMeeting(f); ok {
		activityID := openapi_types.UUID(meeting.ID)
		subject := meeting.Subject
		if subject == "" {
			subject = "meeting"
		}
		return move(ActionOpenMeetingBrief,
			fmt.Sprintf("A meeting is booked %s — read the brief before it.", until(f.now, meeting.StartsAt)),
			map[string]any{argActivityID: activityID},
			crmcontracts.DealNextBestActionEvidence{ActivityId: &activityID, Text: "Booked: " + subject})
	}
	// Existing work is the next step until somebody completes it. Creating
	// another generic follow-up here duplicates the task the card just read.
	if len(f.openTasks) > 0 {
		task := f.openTasks[0]
		activityID := openapi_types.UUID(task.ID)
		return move(ActionOpenTask, "Complete the existing task: "+task.Subject, map[string]any{argActivityID: activityID},
			crmcontracts.DealNextBestActionEvidence{ActivityId: &activityID, Text: task.Subject})
	}
	if request, ok := unansweredInbound(f); ok {
		if request.EmailSummary != nil && request.EmailSummary.RequestHasReminder != nil && *request.EmailSummary.RequestHasReminder {
			return move(ActionDraftEmail, "This request already has a reminder. The reply still needs to be handled.", map[string]any{argActivityID: request.Id},
				evidenceOf(request, "Request: "+subjectOf(request)))
		}
		reason := "Review and take responsibility for the outstanding request: " + subjectOf(request)
		if request.EmailSummary == nil || request.EmailSummary.Move != crmcontracts.EmailSummaryMoveNeedsReply {
			reason = "Review whether this conversation needs a follow-up: " + subjectOf(request)
		}
		return move(ActionCreateTask, reason,
			map[string]any{"subject": subjectOf(request), "request_activity_id": request.Id, "source": "ui"},
			evidenceOf(request, "Request: "+subjectOf(request)))
	}
	if f.deal.Status != crmcontracts.DealStatusOpen {
		return crmcontracts.DealStatusCardMove{
			Action:   ActionNone,
			Reason:   fmt.Sprintf("This deal is %s — there is no next step to take.", f.deal.Status),
			Evidence: []crmcontracts.DealNextBestActionEvidence{},
		}
	}
	// A meeting past the horizon, asked BEFORE the opening move. It is too far
	// off to prepare for, which is why it is not the rung at the top — but it
	// is still an agreed next step, and a deal that has one must not be told to
	// open outreach as though nobody had arranged anything.
	if f.nextMeeting != nil {
		return farMeetingMove(f)
	}
	if opening, ok := firstOutreach(f); ok {
		return opening
	}
	return quietDealMove(f)
}

// farMeetingMove is what to say about a meeting nobody needs to prepare for
// yet: it exists, it is the next step, and there is nothing owed before it.
func farMeetingMove(f facts) crmcontracts.DealStatusCardMove {
	meeting := *f.nextMeeting
	subject := meeting.Subject
	if subject == "" {
		subject = "meeting"
	}
	activityID := openapi_types.UUID(meeting.ID)
	return move(ActionOpenMeetingBrief,
		fmt.Sprintf("A meeting is booked %s — nothing is owed before it.", until(f.now, meeting.StartsAt)),
		map[string]any{argActivityID: activityID},
		crmcontracts.DealNextBestActionEvidence{ActivityId: &activityID, Text: "Booked: " + subject})
}

// quietDealMove is the deal nobody has arranged anything on: open, contacted,
// nothing booked, no human task filed, nobody owed a reply.
//
// The old answer here was "agree the next step", which named nobody and no
// verb the reader could not have worked out themselves. The records already say
// who the deal is with — the seats carry a champion, an economic buyer — so the
// card names them. That is also what the contact page's own rung has always
// said about the same deal, and the two disagreeing on one morning is what sent
// this work here: kernel/nextstep is now the one predicate both ask.
func quietDealMove(f facts) crmcontracts.DealStatusCardMove {
	missing := nextstep.Missing(nextstep.Facts{
		DealOpen:       true,
		MeetingBooked:  f.nextMeeting != nil,
		OpenHumanTasks: len(f.openTasks),
	})
	seat, named := bestSeat(f.seats)
	if !missing || !named {
		// Either something IS agreed, or nobody is recorded on the deal to
		// meet. The generic step stays for both: it names nobody, which is
		// exactly right when the records name nobody.
		return move(ActionCreateTask, nextStepReason(f),
			map[string]any{
				"subject": "Agree the next step on " + f.deal.Name,
				"links":   []map[string]any{{argEntityType: linkDeal, argEntityID: f.deal.Id}},
				"source":  "ui",
			},
			lastContactEvidence(f)...)
	}
	return meetingRequest(f, seat)
}

// meetingRequest files the meeting as work, because filing it is the only verb
// this product has. Nothing in the move vocabulary opens a scheduler, and a
// button that said it books a meeting and did something else would be worse
// than one that files a task saying so.
func meetingRequest(f facts, seat Seat) crmcontracts.DealStatusCardMove {
	links := []map[string]any{{argEntityType: linkDeal, argEntityID: f.deal.Id}}
	// The contact is linked only where this reader may file against them.
	// Reading a contact and adding to their record are different grants, and a
	// link the reader cannot write would fail the POST the button makes — after
	// the click, where the failure looks like the product being broken.
	if seat.Attachable && seat.ContactID != (ids.UUID{}) {
		links = append(links, map[string]any{argEntityType: linkContact, argEntityID: seat.ContactID})
	}
	// A NAMED CONTACT IS WITHHELD RATHER THAN NAMED WITHOUT ITS ID.
	//
	// The sentence and the subject carry this contact's name, and a name is a
	// disclosure on its own. The link is the only place the id survives into
	// the stored card, and the wire shapes are closed — neither the move nor a
	// task body accepts a field of our own — so a move that names somebody it
	// does not link is one the queue can never re-judge: it would keep printing
	// the name after the reader lost the contact, with nothing to check.
	//
	// So the card names a contact only where it also links them. A reader who
	// may read a contact but not file against them gets the role instead, which
	// is the same thing the card says when it may not read the name at all.
	if !seat.Attachable || seat.ContactID == (ids.UUID{}) {
		seat.Name = ""
	}
	return move(ActionCreateTask,
		fmt.Sprintf("The last contact was %s and nothing is booked. Book a meeting with %s.",
			sinceLastContact(f), seatWords(seat)),
		map[string]any{
			"subject": meetingSubject(seat, f.deal),
			"links":   links,
			"source":  "ui",
		},
		lastContactEvidence(f)...)
}

// meetingSubject is what the filed task will be called, in the words the reader
// would have typed: a contact where the card may name one, the role where it
// may not — and then the deal, so the task still says what it is about.
func meetingSubject(seat Seat, deal crmcontracts.Deal) string {
	if seat.Name != "" {
		return "Book a meeting with " + seat.Name
	}
	return fmt.Sprintf("Book a meeting with the %s on %s", roleWord(seat.Role), deal.Name)
}

// sinceLastContact spells how long the deal has been quiet, or says plainly
// that nothing has been logged. A deal with seats and no contact is the
// opening move's business, so this arm is reached only where the timeline is
// readable and empty.
func sinceLastContact(f facts) string {
	last, ok := lastContact(f)
	if !ok {
		return "never"
	}
	return since(f.now, last.OccurredAt)
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
// absent — opening a deal by writing to the contact most likely to refuse it is
// not a first move, and suggesting it would be worse than saying nothing.
var openingRoles = []string{roleChampion, roleEconomicBuyer, roleDecisionMaker, roleInfluencer}

// firstOutreach is the move on a deal nobody has contacted yet.
//
// Without it such a deal was told "Nothing has been logged on this deal yet —
// agree the next step", which restates the empty timeline the reader is
// looking at and names no contact, no role and no verb they could not have
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
	seat, ok := bestSeat(f.seats)
	if !ok {
		return crmcontracts.DealStatusCardMove{}, false
	}
	return move(ActionDraftEmail, openingReason(seat, len(f.seats)), nil), true
}

// openingReason says who to open with and why they are the one.
//
// It carries the seat COUNT because that is the fact a reader checks the
// advice against: "four contacts are named and none has been contacted" is
// checkable against the page, where "reach out" is not.
func openingReason(seat Seat, seats int) string {
	if seats == 1 {
		return fmt.Sprintf("Nobody has been contacted yet. Open with %s.", seatWords(seat))
	}
	return fmt.Sprintf("%d contacts are named on this deal and none has been contacted. Open with %s.",
		seats, seatWords(seat))
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

// upcomingMeeting is the next booked meeting when it is close enough to prepare
// for. It reads the authoritative answer gathered for the deal rather than
// scanning the timeline page, so "nothing is booked" is a fact about the
// calendar instead of a fact about how many rows fit in one page.
//
// A meeting with no status counts as booked — the predicate contact360 and
// company360's next-meeting reads spell — so the card and the record pages
// agree about which meeting is next.
func upcomingMeeting(f facts) (activities.BookedMeeting, bool) {
	if f.nextMeeting == nil {
		return activities.BookedMeeting{}, false
	}
	if f.nextMeeting.StartsAt.After(f.now.Add(meetingHorizon)) {
		return activities.BookedMeeting{}, false
	}
	return *f.nextMeeting, true
}

// unansweredInbound selects from the activities module's obligation read, not
// the recent timeline. An unrelated reply or a history page boundary cannot
// settle a request. The same selection supplies the move and its reply target.
func unansweredInbound(f facts) (crmcontracts.Activity, bool) {
	for _, a := range f.requests {
		if a.Kind == crmcontracts.ActivityKindEmail && !withheld(a) {
			return a, true
		}
	}
	return crmcontracts.Activity{}, false
}

// lastContact is the newest exchange that has already happened. The timeline's
// first rows can be scheduled meetings with future times, which are plans and
// not contact.
func lastContact(f facts) (crmcontracts.Activity, bool) {
	for _, a := range f.timeline {
		if !a.OccurredAt.After(f.now) && relstrength.IsInteractionKind(string(a.Kind)) {
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

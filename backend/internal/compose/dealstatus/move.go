// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package dealstatus

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

	"github.com/margince/margince/backend/internal/compose/activitywhen"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/shared/kernel/dealrole"
	"github.com/margince/margince/backend/internal/shared/kernel/elapsed"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/nextstep"
	"github.com/margince/margince/backend/internal/shared/kernel/occurredday"
	"github.com/margince/margince/backend/internal/shared/kernel/owedwork"
	"github.com/margince/margince/backend/internal/shared/kernel/provenance"
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
	argLinks      = "links"
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
	mv := nextMove(f)
	mv.Basis = requestMoveBasis(f, mv)
	mv.MailCoverage = f.mailCoverage
	if f.lang != "" && f.lang != "en" {
		// Existing dismissal digests include generated English task subjects.
		// Keep that identity while the same rules render the requested language.
		canonical := f
		canonical.lang = "en"
		mv.Fingerprint = decideMove(canonical).Fingerprint
		return mv
	}
	if mv.Fingerprint != nil {
		return mv
	}
	return stamped(mv)
}

func nextMove(f facts) crmcontracts.DealStatusCardMove {
	said := statusCopyFor(f.lang)
	if meeting, ok := upcomingMeeting(f); ok {
		activityID := openapi_types.UUID(meeting.ID)
		subject := meeting.Subject
		if subject == "" {
			subject = said.Say(statusWords.Meeting)
		}
		return move(ActionOpenMeetingBrief,
			fmt.Sprintf(said.Say(statusWords.Prepare), said.until(f.now, meetingDay(meeting, f.loc), f.loc)),
			map[string]any{argActivityID: activityID},
			untimed(&activityID, fmt.Sprintf(said.Say(statusWords.Booked), subject)))
	}
	// A follow-up owed after a meeting outranks open work. The open task is
	// often this card's own generic step, and the buyer waits on the mail.
	if met, ok := followUpOwed(f); ok {
		return followUpMove(f, met)
	}
	// Existing work is the next step until somebody completes it. Creating
	// another generic follow-up here duplicates the task the card just read.
	if len(f.openTasks) > 0 {
		task := f.openTasks[0]
		activityID := openapi_types.UUID(task.ID)
		return move(ActionOpenTask, fmt.Sprintf(said.Say(statusWords.Complete), task.Subject), map[string]any{argActivityID: activityID},
			untimed(&activityID, task.Subject))
	}
	if request, current, ok := requestForMove(f); ok {
		if request.EmailSummary != nil && request.EmailSummary.RequestHasReminder != nil && *request.EmailSummary.RequestHasReminder {
			reason := said.Say(statusWords.Reminder)
			if !current {
				reason = fmt.Sprintf(said.Say(statusWords.ReviewReply), said.subjectOf(request))
			}
			return move(ActionDraftEmail, reason, map[string]any{argActivityID: request.Id},
				evidenceOf(request, fmt.Sprintf(said.Say(statusWords.Request), said.subjectOf(request))))
		}
		reason := fmt.Sprintf(said.Say(statusWords.OwnReply), said.subjectOf(request))
		if !current {
			reason = fmt.Sprintf(said.Say(statusWords.ReviewReply), said.subjectOf(request))
		}
		//nolint:goconst // "source" is the record's provenance KEY, and the Go census
		// (backend/gates/recordsourcespelling_test.go) reads that key as a string literal
		// at each site — a shared constant here would be a key the census cannot see.
		return move(ActionCreateTask, reason,
			map[string]any{"subject": said.subjectOf(request), "request_activity_id": request.Id, "source": provenance.RecordSourceManual},
			evidenceOf(request, fmt.Sprintf(said.Say(statusWords.Request), said.subjectOf(request))))
	}
	if f.deal.Status != crmcontracts.DealStatusOpen {
		return crmcontracts.DealStatusCardMove{
			Action:   ActionNone,
			Reason:   fmt.Sprintf(said.Say(statusWords.Closed), said.dealStatus(string(f.deal.Status))),
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
	said := statusCopyFor(f.lang)
	meeting := *f.nextMeeting
	subject := meeting.Subject
	if subject == "" {
		subject = said.Say(statusWords.Meeting)
	}
	activityID := openapi_types.UUID(meeting.ID)
	return move(ActionOpenMeetingBrief,
		fmt.Sprintf(said.Say(statusWords.FarMeeting), said.until(f.now, meetingDay(meeting, f.loc), f.loc)),
		map[string]any{argActivityID: activityID},
		untimed(&activityID, fmt.Sprintf(said.Say(statusWords.Booked), subject)))
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
	said := statusCopyFor(f.lang)
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
				"subject": fmt.Sprintf(said.Say(statusWords.Agree), f.deal.Name),
				argLinks:  []map[string]any{{argEntityType: linkDeal, argEntityID: f.deal.Id}},
				"source":  provenance.RecordSourceManual,
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
	said := statusCopyFor(f.lang)
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
		fmt.Sprintf(said.Say(statusWords.Book),
			sinceLastContact(f), said.seatWords(seat)),
		map[string]any{
			"subject": said.meetingSubject(seat, f.deal),
			argLinks:  links,
			"source":  provenance.RecordSourceManual,
		},
		lastContactEvidence(f)...)
}

// meetingSubject is what the filed task will be called, in the words the reader
// would have typed: a contact where the card may name one, the role where it
// may not — and then the deal, so the task still says what it is about.
func (said statusCopy) meetingSubject(seat Seat, deal crmcontracts.Deal) string {
	if seat.Name != "" {
		return fmt.Sprintf(said.Say(statusWords.BookNamed), seat.Name)
	}
	return fmt.Sprintf(said.Say(statusWords.BookRole), said.roleWord(seat.Role), deal.Name)
}

// sinceLastContact spells how long the deal has been quiet, or says plainly
// that nothing has been logged. A deal with seats and no contact is the
// opening move's business, so this arm is reached only where the timeline is
// readable and empty.
func sinceLastContact(f facts) string {
	said := statusCopyFor(f.lang)
	last, ok := lastContact(f)
	if !ok {
		return said.Say(statusWords.Never)
	}
	return said.since(f.now, activitywhen.MomentOf(last), f.loc)
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
	roleDecisionMaker = dealrole.LegacyDecisionMaker
	roleInfluencer    = dealrole.Influencer
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
	said := statusCopyFor(f.lang)
	if _, contacted := lastContact(f); contacted {
		return crmcontracts.DealStatusCardMove{}, false
	}
	seat, ok := bestSeat(f.seats)
	if !ok {
		return crmcontracts.DealStatusCardMove{}, false
	}
	// The stakeholder lives only in the reason, so the digest names them: "open
	// with Alice" set aside must not hide "open with Bob".
	opening := move(ActionDraftEmail, said.openingReason(seat, len(f.seats)), nil)
	return stamped(opening, owedwork.Mark{Kind: "seat", ID: seat.Role + ":" + seat.ContactID.String()}), true
}

// openingReason says who to open with and why they are the one.
//
// It carries the seat COUNT because that is the fact a reader checks the
// advice against: "four contacts are named and none has been contacted" is
// checkable against the page, where "reach out" is not.
func (said statusCopy) openingReason(seat Seat, seats int) string {
	if seats == 1 {
		return fmt.Sprintf(said.Say(statusWords.OpeningOne), said.seatWords(seat))
	}
	return fmt.Sprintf(said.Say(statusWords.OpeningMany),
		seats, said.seatWords(seat))
}

// roleWord is a stakeholder role as a sentence says it. An unknown role is
// returned as it is stored rather than dropped: the card would otherwise write
// "Open with Maria Schmidt, the ." for a role added after this build.
func (said statusCopy) roleWord(role string) string {
	return dealrole.Word(role, string(said.Lang()))
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

// The activities owner admits current requests before limiting the result.
func unansweredInbound(f facts) (crmcontracts.Activity, bool) {
	for _, a := range f.currentRequests {
		if a.Kind == crmcontracts.ActivityKindEmail && !withheld(a) {
			return a, true
		}
	}
	return crmcontracts.Activity{}, false
}

// lastContact is the newest exchange that has already happened. The timeline's
// first rows can be scheduled meetings, which are plans and not contact. A
// canceled or no-show meeting and automatic mail are no contact either.
func lastContact(f facts) (crmcontracts.Activity, bool) {
	for _, a := range f.timeline {
		var status string
		if a.MeetingStatus != nil {
			status = string(*a.MeetingStatus)
		}
		var automatic relstrength.AutomaticKind
		if a.AutomaticKind != nil {
			automatic = relstrength.AutomaticKind(*a.AutomaticKind)
		}
		if occurredday.Happened(string(a.Kind), activitywhen.MomentOf(a), f.now, f.loc) &&
			relstrength.InteractionCounts(string(a.Kind), status, automatic) {
			return a, true
		}
	}
	return crmcontracts.Activity{}, false
}

func nextStepReason(f facts) string {
	said := statusCopyFor(f.lang)
	last, ok := lastContact(f)
	if !ok {
		return said.Say(statusWords.NoContactNext)
	}
	return fmt.Sprintf(said.Say(statusWords.QuietNext),
		said.since(f.now, activitywhen.MomentOf(last), f.loc))
}

func lastContactEvidence(f facts) []crmcontracts.DealNextBestActionEvidence {
	said := statusCopyFor(f.lang)
	last, ok := lastContact(f)
	if !ok {
		return nil
	}
	return []crmcontracts.DealNextBestActionEvidence{evidenceOf(last, fmt.Sprintf(said.Say(statusWords.LastEvidence), said.subjectOf(last)))}
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
	out := untimed(&id, text)
	if !a.OccurredAt.IsZero() {
		at := a.OccurredAt
		out.OccurredAt, out.OccurredPrecision = &at, a.OccurredPrecision
	}
	return out
}

// untimed cites a record by id alone. The wire reads a row with no time as an
// instant.
func untimed(id *openapi_types.UUID, text string) crmcontracts.DealNextBestActionEvidence {
	return crmcontracts.DealNextBestActionEvidence{
		ActivityId: id, Text: text, OccurredPrecision: crmcontracts.OccurredPrecisionOccurredPrecisionInstant,
	}
}

// subjectOf names a row for a reader. A withheld row is named by its kind:
// the reader may know contact happened without reading what was said.
func (said statusCopy) subjectOf(a crmcontracts.Activity) string {
	if withheld(a) || a.Subject == nil || *a.Subject == "" {
		switch a.Kind {
		case crmcontracts.ActivityKindCall:
			return said.Say(statusWords.Call)
		case crmcontracts.ActivityKindEmail:
			return said.Say(statusWords.Email)
		case crmcontracts.ActivityKindMeeting:
			return said.Say(statusWords.Meeting)
		case crmcontracts.ActivityKindMessage:
			return said.Say(statusWords.Message)
		case crmcontracts.ActivityKindNote:
			return said.Say(statusWords.Note)
		case crmcontracts.ActivityKindTask:
			return said.Say(statusWords.Task)
		default:
			return string(a.Kind)
		}
	}
	return *a.Subject
}

// since spells the days from an activity to now. A day entry counts from its
// own date, not from where its stored noon falls in loc.
func (said statusCopy) since(now time.Time, then occurredday.Moment, loc *time.Location) string {
	return said.spell(occurredday.DaysSince(then, now, loc))
}

// meetingDay is when the booked meeting falls for a day count. A day meeting
// counts by its own date, not by where its stored noon falls in loc.
func meetingDay(meeting activities.BookedMeeting, loc *time.Location) time.Time {
	if !occurredday.IsDay(meeting.Precision) {
		return meeting.StartsAt
	}
	return occurredday.Begins(meeting.StartsAt, loc)
}

func (said statusCopy) until(now, then time.Time, loc *time.Location) string {
	days := elapsed.Days(now, then, loc)
	switch days {
	case 0:
		return said.Say(statusWords.Today)
	case 1:
		return said.Say(statusWords.Tomorrow)
	default:
		return fmt.Sprintf(said.Say(statusWords.InDays), days)
	}
}

func (said statusCopy) spell(days int) string {
	switch {
	case days <= 0:
		return said.Say(statusWords.Today)
	case days == 1:
		return said.Say(statusWords.Yesterday)
	default:
		return fmt.Sprintf(said.Say(statusWords.DaysAgo), days)
	}
}

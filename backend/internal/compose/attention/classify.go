// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package attention

// What each item IS, in the queue's own terms: which band it belongs to, why it
// is there, and what happens if the reader does nothing.
//
// This is the editorial layer the lane feed does not have. A lane says "this
// came from the bounce reader"; a queue has to say "a customer never received
// your quote, and nobody else is going to notice".
//
// The consequence is derived per ITEM rather than per source, because one source
// has several honest answers: a deal past its close date SLIPS, while one merely
// idle DRIFTS, and a reader who is told the wrong one stops believing the
// right ones.

import (
	"time"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
)

// sourceWaiting names the who-is-waiting producer. A named constant rather
// than the literal at each site: the classifier, the dedupe and the
// source-unavailable report all reach for it, and a typo in any of them would
// produce a lane nothing joins up — silently, because each half would still
// compile.
const sourceWaiting = "customer_waiting"

// sourceTask names the open-task producer. Named for the reason sourceWaiting
// is: the owner filter asks whether a row came from the lane that narrowed to
// one person in its own query, and a typo there would silently drop every task
// out of the queue it was asked for.
const sourceTask = "task"

// sourceClaim names the rep's own promises. keepUnowned reads it: a claim has
// no assignee column, so the row arrives ownerless while already belonging to
// the rep whose query produced it.
const sourceClaim = "conversation_claim"

// sourceAtRisk names the quiet-deal producer. Three readers spell it — the
// bounds table, the category map and the classifier — which is two more than a
// literal survives.
const sourceAtRisk = "deal_at_risk"

// subjectDeal is the subject type a deal-shaped row names.
const subjectDeal = "deal"

// subjectPerson is the subject type a person-shaped row names.
//
// A constant for the reason sourceDecay is one: the suppressor pairing the
// decay lane against a waiting row matches on it, and a misspelt literal there
// fails silently — it matches nothing, drops nothing, and reads green.
const subjectPerson = "person"

// classifyDay turns the assembled lanes into ranked candidates.
//
// Order of appearance does not matter — rankAll decides the order — so the lanes
// are walked in whatever order reads clearest here.
func classifyDay(day crmcontracts.Attention, asOf time.Time, money dayMoney) []ranked {
	rows := make([]ranked, 0, 64)
	bar := materialBarOf(day, money)
	rows = appendLane(rows, day.Meetings, asOf, classifyMeeting)
	rows = appendLane(rows, &day.ThisMorning, asOf, classifyBriefItem)
	rows = appendLane(rows, day.Commitments, asOf, classifyCommitment)
	rows = appendLane(rows, day.DidNotRun, asOf, classifyFailedApproval)
	rows = appendLane(rows, day.Dsr, asOf, classifyDSR)
	rows = appendLane(rows, day.AtRisk, asOf, func(item crmcontracts.AttentionItem, at time.Time) ranked {
		return classifyRisk(item, at, bar, money)
	})
	rows = appendLane(rows, &day.Planned, asOf, classifyTask)
	rows = appendLane(rows, day.Bounces, asOf, classifyBounce)
	rows = appendLane(rows, day.Undelivered, asOf, classifyUndelivered)
	rows = appendLane(rows, &day.NeedsYou, asOf, classifyDecision)
	rows = appendLane(rows, day.RelationshipDecay, asOf, classifyDecay)
	rows = appendLane(rows, day.CaptureHealth, asOf, classifySystem)
	rows = appendLane(rows, day.AiWorkHealth, asOf, classifySystem)
	rows = appendLane(rows, day.AutomationHealth, asOf, classifySystem)
	rows = appendLane(rows, day.SyncHealth, asOf, classifySystem)
	rows = appendLane(rows, day.Notices, asOf, classifySystem)
	rows = appendLane(rows, day.Introductions, asOf, classifyIntroduction)
	return rows
}

// appendLane classifies one lane, skipping a lane the feed did not serve.
func appendLane(
	rows []ranked,
	lane *[]crmcontracts.AttentionItem,
	asOf time.Time,
	classify func(crmcontracts.AttentionItem, time.Time) ranked,
) []ranked {
	if lane == nil {
		return rows
	}
	for _, item := range *lane {
		rows = append(rows, classify(item, asOf))
	}
	return rows
}

// base carries across every fact the two feeds share, so a field the lane feed
// already resolved — a title, a subject, an overdue flag — is never re-derived
// here and never allowed to disagree with the card the same item draws there.
func base(
	item crmcontracts.AttentionItem,
	level int,
	category crmcontracts.WorklistItemCategory,
	consequence crmcontracts.WorklistItemConsequence,
) crmcontracts.WorklistItem {
	return crmcontracts.WorklistItem{
		Id:          item.Id,
		Source:      crmcontracts.WorklistItemSource(item.Source),
		Category:    category,
		Level:       level,
		Consequence: consequence,
		Kind:        item.Kind,
		Title:       item.Title,
		Detail:      item.Detail,
		CauseRef:    item.CauseRef,
		// The identity AND the words for it. The identity groups the row; the
		// label is what the group says. Forwarding only the first is how the
		// client came to interpolate an identity into a sentence.
		CauseLabel: item.CauseLabel,
		Subject:    item.Subject,
		Deal:       dealFactsOf(item),
		// Forwarded, never re-derived here. The lane already applied the
		// both-sides-visible rule and set `merge` only where it held, so
		// carrying the payload keeps the verb and the records it acts on
		// travelling together: a row offering merge with no pair beneath it
		// would be a button over records the client cannot name.
		Pair:       item.Pair,
		DueAt:      item.DueAt,
		Overdue:    item.Overdue,
		OccurredAt: item.OccurredAt,
		Actions:    carriedActions(item.Actions),
		Because:    []crmcontracts.WorklistReason{},
	}
}

// carriedActions passes the lane feed's verbs through unchanged. The queue adds
// no authority of its own: every verb still routes to the endpoint that owns it.
func carriedActions(actions []crmcontracts.AttentionItemActions) []crmcontracts.WorklistItemActions {
	out := make([]crmcontracts.WorklistItemActions, 0, len(actions))
	for _, action := range actions {
		out = append(out, crmcontracts.WorklistItemActions(action))
	}
	return out
}

// classifyCommitment: a promise the rep made. Level 2 whether or not it is
// overdue — the promise is the fact, and the date only orders it.
func classifyCommitment(item crmcontracts.AttentionItem, asOf time.Time) ranked {
	row := base(item, levelPromise, "tasks", "promise_breaks")
	stampDeadline(&row, item.DueAt, asOf)
	row.Because = []crmcontracts.WorklistReason{reason("promised", nil)}
	if overdueAt(item.DueAt, asOf) {
		row.Because = append(row.Because, reason("overdue", nil))
	}
	return ranked{
		item:       row,
		deadlineAt: deadlineOf(item.DueAt),
		overdue:    overdueAt(item.DueAt, asOf),
		occurredAt: occurredOf(item, asOf),
	}
}

// classifyFailedApproval: the rep pressed Accept and believes it happened. That
// belief is the damage, which is why it sits beside a broken promise rather
// than with the other system news.
func classifyFailedApproval(item crmcontracts.AttentionItem, asOf time.Time) ranked {
	row := base(item, levelPromise, "system", "you_believe_it_happened")
	row.Because = []crmcontracts.WorklistReason{reason("approved_and_failed", nil)}
	return ranked{item: row, occurredAt: occurredOf(item, asOf)}
}

// classifyIntroduction: a colleague is waiting on this reader to answer.
//
// levelBlocking, which is "a decision that holds up customer work", because
// that is precisely what it is: a rep's deal is stopped until this colleague
// says yes, no, or ask somebody else. It is a DECISION rather than system news
// — a person must choose, and only this person can.
//
// The deadline is stamped like the DSR's, because both are somebody else's
// clock running and the queue orders by it. An ask that lapses reads to the
// requester exactly like a refusal, and the difference is whether anybody
// looked in time.
func classifyIntroduction(item crmcontracts.AttentionItem, asOf time.Time) ranked {
	row := base(item, levelBlocking, "decisions", "work_blocked")
	stampDeadline(&row, item.DueAt, asOf)
	row.Because = []crmcontracts.WorklistReason{reason("blocks_customer_work", nil)}
	return ranked{
		item:       row,
		deadlineAt: deadlineOf(item.DueAt),
		overdue:    overdueAt(item.DueAt, asOf),
		occurredAt: occurredOf(item, asOf),
	}
}

// classifyDSR: a clock the law started. It reaches only privacy admins — the
// lane is absent for everyone else — so it never needs explaining to a rep.
func classifyDSR(item crmcontracts.AttentionItem, asOf time.Time) ranked {
	row := base(item, levelWaiting, "system", "legal_deadline_missed")
	stampDeadline(&row, item.DueAt, asOf)
	row.Because = []crmcontracts.WorklistReason{reason("legal_deadline", nil)}
	return ranked{
		item:       row,
		deadlineAt: deadlineOf(item.DueAt),
		overdue:    overdueAt(item.DueAt, asOf),
		occurredAt: occurredOf(item, asOf),
	}
}

// waitingStaleDays is when an unanswered message stops being today's work.
//
// Past it the wait is still real, but acting on it is no longer urgent in the
// way the top band means: two weeks of silence has already cost whatever it was
// going to cost, and a fortnight-old row sitting above a meeting in an hour is
// the page lying about what matters now. Such a row moves to the review band —
// still visible, still answerable, no longer claiming the day.
//
// Unless money is still on it. An open deal keeps a long wait in execution,
// because there the silence is the problem rather than a closed chapter.
const waitingStaleDays = 14

// waitingDaysCeiling bounds what age contributes to the ORDER.
//
// Age breaks ties between rows the bands could not separate; it does not earn
// precedence on its own. Uncapped it does exactly that — every additional day
// of silence outranks every newer wait forever, so the oldest thread in the
// workspace leads the page permanently and the queue becomes an archive sorted
// by how long it has been ignored. That is the live page's own defect: eight
// half-year-old threads holding the top of a working rep's day.
//
// Past the ceiling all waits tie on age and the next tie-break decides, which
// is the honest answer — at six months versus seven, age has stopped saying
// anything about what to do first.
const waitingDaysCeiling = 30

// classifyWaiting: somebody wrote and nobody answered.
//
// Level 1, the top band below a pin, and the reason is the concept's own: a
// customer waiting on a reply is the one thing on this page where the cost of
// doing nothing falls on somebody else. It outranks a promise, a drifting deal
// and every decision.
//
// That top band is for a LIVE wait. A stale one keeps its row and loses the
// band, because a queue where nothing ever ages out is one a rep stops reading.
//
// The draft verb is offered only when the message can be READ. There is no
// drafting a reply to words this reader may not see, and a button that opened
// an empty composer would be worse than no button.
func classifyWaiting(waiting WaitingCustomer, asOf time.Time) ranked {
	subject := waiting.Subject
	days := daysSince(waiting.Since, asOf)
	// Stale and unfunded: the row belongs to review, not to today.
	level := levelWaiting
	stale := days > waitingStaleDays && !waiting.HasOpenDeal
	if stale {
		level = levelRoutine
	}
	because := []crmcontracts.WorklistReason{
		reason("buyer_wrote_last", nil),
		reason("waiting_days", daysValue(days)),
	}
	if stale {
		because = append(because, reason("stale", nil))
	}
	row := crmcontracts.WorklistItem{
		Id:          waiting.ActivityID.String(),
		Source:      sourceWaiting,
		Category:    sourceWaiting,
		Level:       level,
		Consequence: "buyer_waits",
		Because:     because,
		Actions:     []crmcontracts.WorklistItemActions{},
	}
	// The subject travels because the row exists at all only for a reader the
	// content gate admitted: a message this reader may not read produces no
	// row, rather than a row with its words removed.
	if subject != "" {
		row.Title = &subject
	}
	// Present exactly when this wait is an email the reader may read. A client
	// branches on the field rather than on the kind word: the lane also carries
	// channel messages, and each keeps the plain title it had.
	row.EmailSummary = waiting.EmailSummary
	// The record the reply would be about, most specific first: the deal a
	// thread belongs to says more than the company it is filed under.
	switch {
	case !waiting.DealID.IsZero():
		row.Subject = subjectOf(subjectDeal, waiting.DealID)
	case !waiting.PersonID.IsZero():
		row.Subject = subjectOf("person", waiting.PersonID)
	case !waiting.OrganizationID.IsZero():
		row.Subject = subjectOf("organization", waiting.OrganizationID)
	}
	if openableSubject(row.Subject) {
		row.Actions = append(row.Actions, crmcontracts.WorklistItemActions(actionOpen))
	}
	occurred := waiting.Since
	row.OccurredAt = &occurred
	// The message IS the row's id, so the move needs nothing this row does not
	// already carry. The product knew the buyer had written and knew nobody had
	// answered; naming the step is the difference between a page that reports
	// and a page a rep can work from.
	answers := openapi_types.UUID(waiting.ActivityID)
	row.Move = &crmcontracts.WorklistMove{
		Action:     crmcontracts.WorklistMoveActionDraftReply,
		ActivityId: &answers,
	}
	// Both ages travel: the true one for everything a reader is shown, the
	// bounded one for the order. A rep reading "waiting 180 days" is being told
	// something true; the queue placing that row above everything for the rest
	// of its life is not.
	ordering := days
	if ordering > waitingDaysCeiling {
		ordering = waitingDaysCeiling
	}
	return ranked{
		item:        row,
		waitingDays: days,
		waitingRank: ordering,
		occurredAt:  waiting.Since,
		// Who owes the reply, so the scope filters can judge this row the way
		// they judge a deal-bearing one. A wait carries no deal on the wire, and
		// without this it is a row the filters cannot place: a named owner's
		// queue dropped every one of them.
		owner: waiting.OwnerID,
	}
}

// dropDealsAlreadyWaiting removes the at-risk row for a deal somebody is
// already waiting on.
//
// One unanswered message must not become two rows. The waiting row is strictly
// the more urgent and the more actionable of the two — it names the message to
// reply to — so it wins, and the drifting row's ground rides along as a reason
// rather than as a second obligation.
func dropDealsAlreadyWaiting(rows []ranked) []ranked {
	waitingDeals := map[string]bool{}
	for _, row := range rows {
		if row.item.Source == sourceWaiting && row.item.Subject != nil &&
			row.item.Subject.Type == subjectDeal {
			waitingDeals[row.item.Subject.Id.String()] = true
		}
	}
	if len(waitingDeals) == 0 {
		return rows
	}
	// The deal's own facts ride onto the waiting row that absorbed it: a reader
	// told a customer is waiting still wants to know the deal is worth €160k.
	// Everything the drifting row was going to say, kept: a reader told a
	// customer is waiting still needs to know the deal is material, has been
	// quiet a month, and is past the date it was meant to close.
	facts := map[string]*crmcontracts.WorklistDealFacts{}
	grounds := map[string][]crmcontracts.WorklistReason{}
	for _, row := range rows {
		if row.item.Source == "deal_at_risk" && waitingDeals[row.item.Id] {
			facts[row.item.Id] = row.item.Deal
			grounds[row.item.Id] = row.item.Because
		}
	}
	kept := make([]ranked, 0, len(rows))
	for _, row := range rows {
		if row.item.Source == "deal_at_risk" && waitingDeals[row.item.Id] {
			continue
		}
		if row.item.Source == sourceWaiting && row.item.Subject != nil &&
			row.item.Subject.Type == subjectDeal && row.item.Deal == nil {
			deal := row.item.Subject.Id.String()
			row.item.Deal = facts[deal]
			row.item.Because = append(row.item.Because, grounds[deal]...)
		}
		kept = append(kept, row)
	}
	return kept
}

// classifyTask: work already agreed. Overdue is the fact that moves it; a task
// nobody dated is real work and is not today's.
func classifyTask(item crmcontracts.AttentionItem, asOf time.Time) ranked {
	row := base(item, levelAgreed, "tasks", "task_slips")
	stampDeadline(&row, item.DueAt, asOf)
	if overdueAt(item.DueAt, asOf) {
		row.Because = append(row.Because, reason("overdue", nil))
	} else if item.DueAt != nil {
		row.Because = append(row.Because, reason("due_today", nil))
	}
	// Nobody has taken it. The same fact the lead lane states, and this lane
	// has a whole scope devoted to surfacing it — a sweep of unowned work whose
	// rows could not say that was what they were.
	if item.AssigneeId == nil {
		row.Because = append(row.Because, reason("unassigned", nil))
	}
	return ranked{
		item:       row,
		deadlineAt: deadlineOf(item.DueAt),
		overdue:    overdueAt(item.DueAt, asOf),
		occurredAt: occurredOf(item, asOf),
	}
}

// classifyBounce: a customer never received something the rep believes they
// sent. A consequence with a named customer and a verb, so it ranks as its own
// row rather than disappearing into an aggregate.
func classifyBounce(item crmcontracts.AttentionItem, asOf time.Time) ranked {
	row := base(item, levelPromise, "system", "customer_never_received")
	row.Because = []crmcontracts.WorklistReason{reason("approved_and_failed", nil)}
	return ranked{item: row, occurredAt: occurredOf(item, asOf)}
}

// classifyUndelivered: a message the rep believes they sent and which never
// left. The belief is the damage — they are waiting on a reply to something
// nobody has — so it sits with the broken promises rather than with the system
// news, exactly where a bounce sits.
//
// It is a separate consequence from the bounce beside it: nobody received this
// one because nobody was ever sent it, and the reader's move is to send it
// rather than to fix an address.
func classifyUndelivered(item crmcontracts.AttentionItem, asOf time.Time) ranked {
	row := base(item, levelPromise, "system", "you_believe_it_happened")
	row.Because = []crmcontracts.WorklistReason{reason("approved_and_failed", nil)}
	return ranked{item: row, occurredAt: occurredOf(item, asOf)}
}

// classifySystem: the pipes. A mailbox that stopped capturing makes every quiet
// claim on this page suspect, which is why it is here at all rather than only in
// an admin screen.
func classifySystem(item crmcontracts.AttentionItem, asOf time.Time) ranked {
	consequence := crmcontracts.WorklistItemConsequence("work_blocked")
	if item.Source == "capture_health" || item.Source == "sync_health" {
		consequence = "mailbox_blind"
	}
	if item.Source == "notice" {
		consequence = valueNone
	}
	row := base(item, levelBlocking, "system", consequence)
	return ranked{item: row, occurredAt: occurredOf(item, asOf)}
}

// stampDeadline marks an item whose date the READER owes.
//
// The flag is what tells a real deadline from a staged proposal's expiry: the
// producers whose date is a deadline resolve it here, and the ones whose date
// is a lapse moment leave it unset, so the header can count work due without
// counting proposals that merely go stale.
func stampDeadline(row *crmcontracts.WorklistItem, due *time.Time, asOf time.Time) {
	if due == nil {
		return
	}
	past := overdueAt(due, asOf)
	row.Overdue = &past
}

func reason(kind crmcontracts.WorklistReasonKind, value *crmcontracts.WorklistValue) crmcontracts.WorklistReason {
	return crmcontracts.WorklistReason{Kind: kind, Value: value}
}

func deadlineOf(due *time.Time) time.Time {
	if due == nil {
		return time.Time{}
	}
	return *due
}

// occurredOf answers when the reported thing happened, falling back to the read
// instant so a row with no timestamp sorts as "now" rather than as 1 January
// year one — which would put every undated row at the head of its level.
func occurredOf(item crmcontracts.AttentionItem, asOf time.Time) time.Time {
	if item.OccurredAt != nil {
		return *item.OccurredAt
	}
	return asOf
}

// quietDaysOf reads the idle count the risk and decay lanes measure.
//
// From the TYPED field, not parsed out of the supporting sentence. It used to
// read `detail` a digit at a time and answer zero for anything else, which made
// the ordering depend on a display string: a lane that made its sentence
// friendlier — "quiet for 90 days" instead of "90" — would have dropped every
// such row to the bottom of the queue, and nothing would have failed.
//
// A lane that measures no idle time sends none, and zero is what that means.
func quietDaysOf(item crmcontracts.AttentionItem) int {
	if item.QuietDays == nil {
		return 0
	}
	return *item.QuietDays
}

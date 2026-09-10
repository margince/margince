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

// classifyDay turns the assembled lanes into ranked candidates.
//
// Order of appearance does not matter — rankAll decides the order — so the lanes
// are walked in whatever order reads clearest here.
func classifyDay(day crmcontracts.Attention, asOf time.Time, money dayMoney) []ranked {
	rows := make([]ranked, 0, 64)
	bar := materialBarOf(day, money)
	rows = appendLane(rows, day.Meetings, asOf, classifyMeeting)
	rows = appendLane(rows, day.MeetingsUnreported, asOf, classifyUnansweredMeeting)
	rows = appendLane(rows, &day.ThisMorning, asOf, func(item crmcontracts.AttentionItem, at time.Time) ranked {
		return classifyBriefItem(item, at, money)
	})
	rows = appendLane(rows, day.Commitments, asOf, classifyCommitment)
	rows = appendLane(rows, day.DidNotRun, asOf, classifyFailedApproval)
	rows = appendLane(rows, day.Dsr, asOf, classifyLegalDeadline)
	rows = appendLane(rows, day.NoticeCases, asOf, classifyLegalDeadline)
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
		// The row the card's own verbs write to, forwarded like every other fact
		// the lane already resolved. A worklist row that offers `complete` and
		// then cannot pin the write is the last-write-wins this field ends.
		Version: item.Version,
		Deal:    dealFactsOf(item),
		// Whose page a meeting's brief opens on. Forwarded rather than derived
		// here: the lane already decided whether the reader may see anybody on
		// the meeting, and an absent value is that decision rather than a gap.
		WithPerson: item.WithPerson,
		// Forwarded, never re-derived here. The lane already applied the
		// both-sides-visible rule and set `merge` only where it held, so
		// carrying the payload keeps the verb and the records it acts on
		// travelling together: a row offering merge with no pair beneath it
		// would be a button over records the client cannot name.
		Pair:    item.Pair,
		DueAt:   item.DueAt,
		Overdue: item.Overdue,
		// Carried rather than recomputed, for the reason the deadline itself
		// is: the group was resolved against the assembly's own boundary and
		// zone, and a second derivation here would need both again.
		DueGroup: carriedDueGroup(item.DueGroup),
		// Carried, not re-derived: the lane decided whether there was a way
		// back, and a queue that answered it again could offer the verb on a
		// row the lane had already reversed.
		Undo:       carriedUndo(item.Undo),
		OccurredAt: item.OccurredAt,
		Actions:    carriedActions(item.Actions),
		Because:    []crmcontracts.WorklistReason{},
	}
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
		ownerRef:   ownedByWhoeverIsReading(),
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
	return ranked{
		item:       row,
		occurredAt: occurredOf(item, asOf),
		// Carried back to the person who APPROVED it, by a lane bound to them.
		ownerRef: ownedByWhoeverIsReading(),
	}
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
		ownerRef:   ownedByWhoeverIsReading(),
		item:       row,
		deadlineAt: deadlineOf(item.DueAt),
		overdue:    overdueAt(item.DueAt, asOf),
		occurredAt: occurredOf(item, asOf),
	}
}

// classifyLegalDeadline: a clock the law started, and the shape BOTH compliance
// lanes take. A subject request and a disclosure duty differ in what they oblige
// and in nothing this function decides — same band, same reason, same deadline
// stamp, same unassigned owner — so they share it rather than drifting apart.
//
// UNASSIGNED, for both. Each is a compliance queue rather than a personal one:
// the lane reads every open item due soonest behind one gate, so several admins
// see the same row and none owns it by having looked. A subject request has no
// assignee column at all; a notice case has a nullable owner that is frequently
// empty, and reading it as an assignment would hide an unowned overdue duty from
// everybody — the failure the lane exists to prevent.
//
// Both lanes reach only privacy admins — they are absent for everyone else — so
// neither ever needs explaining to a rep.
func classifyLegalDeadline(item crmcontracts.AttentionItem, asOf time.Time) ranked {
	row := base(item, levelWaiting, "system", "legal_deadline_missed")
	stampDeadline(&row, item.DueAt, asOf)
	row.Because = []crmcontracts.WorklistReason{reason("legal_deadline", nil)}
	return ranked{
		ownerRef:   unassigned(),
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
	// Nobody here has written on this thread, and no money is on it either.
	//
	// DEMOTED, never dropped. The evidence is thread identity, which comes from
	// the sender's reply headers: a client that strips them gives every message
	// its own thread, and this reads as "we never spoke" about a conversation
	// three replies deep. Hiding on that would lose a live customer with nothing
	// on the page to say so — the one failure this queue must not have — so the
	// cost of being wrong is a scroll instead.
	//
	// Money outranks it, exactly as it does for staleness: an open deal on the
	// thread is a stronger claim than any header the sender chose to send.
	unproven := !waiting.Engaged && !waiting.HasOpenDeal
	if unproven {
		level = levelRoutine
	}
	// Written to somebody else.
	//
	// A reader sees mail addressed to a colleague — copied in, or on a record
	// they own — and the queue called all of it a customer waiting on THEM. The
	// header is the evidence: capture stamps this mailbox's owner as a
	// recipient on every message it stores, so the participant row proves only
	// that the mail arrived.
	//
	// NO MONEY OVERRIDE, which is what separates this from every demotion
	// around it. Those ask whether a wait MATTERS, and an open deal is a
	// stronger claim than a header. This asks WHOSE it is, and a deal on the
	// thread does not make a colleague's mail into this reader's reply to
	// write — the colleague has the same row on their own queue, where it is
	// addressed to them and ranks accordingly.
	elsewhere := waiting.AddressedElsewhere
	if elsewhere {
		level = levelRoutine
	}
	// A message that asks us nothing. A report, a receipt, a statement: the
	// sender wrote and nobody replied, both true, and neither makes it work.
	//
	// DEMOTED, never dropped — the same floor capture_label sits under, and for
	// a sharper reason here: this is one model call's opinion about a customer's
	// mail. A wrong one costs a scroll. An UNJUDGED message is not demoted at
	// all: a classifier that never ran, ran out of budget or answered below its
	// confidence floor must leave the queue exactly as it found it.
	//
	// Money outranks it, like every other demotion here.
	informational := waiting.AsksNothing && !waiting.HasOpenDeal
	if informational {
		level = levelRoutine
	}
	because := []crmcontracts.WorklistReason{
		reason("buyer_wrote_last", nil),
		reason("waiting_days", daysValue(days)),
	}
	if stale {
		because = append(because, reason("stale", nil))
	}
	if elsewhere {
		because = append(because, reason("addressed_elsewhere", nil))
	}
	if unproven {
		because = append(because, reason("no_reply_history", nil))
	}
	if informational {
		because = append(because, reason("asks_nothing", nil))
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
		// Answering where the reader is standing, offered only for an EMAIL. The
		// lane also carries channel messages, and the composer this verb opens
		// speaks mail — a chat message it addressed would be answered in the
		// wrong place, to a counterparty resolved from a thread that is not one.
		//
		// `email_summary` is the honest test rather than the kind word: the
		// contract sets it exactly when the wait is mail, and it is absent for a
		// reader the content gate did not admit — who has no message to answer.
		//
		// It rides the same subject test as `open`, because the composer files
		// the reply against that record. A wait with no subject would open a
		// composer with nothing to link the sent message to.
		if row.EmailSummary != nil {
			row.Actions = append(row.Actions, crmcontracts.WorklistItemActionsReply)
		}
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
	return ranked{
		item:        row,
		waitingDays: days,
		waitingRank: orderingAge(days),
		occurredAt:  waiting.Since,
		// Who owes the reply, so the scope filters can judge this row the way
		// they judge a deal-bearing one. A wait carries no deal on the wire, and
		// without this it is a row the filters cannot place: a named owner's
		// queue dropped every one of them.
		owner: waiting.OwnerID,
		// The same fact for the CLIENT — but NOT through ownerFrom, because a
		// zero here does not mean what a zero means to the task and lead lanes.
		// This lane qualifies a row through an ungated lookup and reads its
		// owner through a gated one, so a customer whose owning record the
		// reader may not open arrives with no owner id at all. Reporting that
		// as `unassigned` would turn a withheld fact into a claim that nobody
		// owes the reply, and the reader has no way to tell the two apart.
		ownerRef: waitingOwner(waiting.OwnerID),
		// And WHO it is about, which the subject above may have given to a deal.
		// The decay suppressor reads this rather than the subject, so a contact
		// whose wait is filed under a deal is still recognised as answered.
		person: waiting.PersonID,
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
	} else if item.DueAt != nil && !isUpcoming(item.DueGroup) {
		// Only work that IS due today says so. The lane now also carries what
		// is coming, and a task due next week telling the reader it is due
		// today contradicts the group on the same row.
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
		// The assignee the lane read, which is the same fact the reason above
		// states in words. Absent means nobody has taken it — the state the
		// unassigned scope exists to surface — and the two must agree: a row
		// saying "unassigned" in its reasons while naming an owner beside them
		// is a row a reader cannot make sense of.
		ownerRef: ownerFromAssignee(item.AssigneeId),
		// And the SAME id where the scope filters read it. Without this a task
		// answers nobody to answersTo, so keepTeams keeps it as unowned work —
		// which is the hole its own comment describes for link-less tasks, and
		// which now also puts an outside-team colleague's user id on the wire
		// through the owner field. One assignee, read by both.
		owner: assigneeID(item.AssigneeId),
	}
}

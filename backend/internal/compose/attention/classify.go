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
	rows = appendLane(rows, day.DomainQuestions, asOf, classifyDomainQuestion)
	rows = appendLane(rows, day.AiWorkHealth, asOf, classifySystem)
	rows = appendLane(rows, day.AutomationHealth, asOf, classifySystem)
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
		Id:           item.Id,
		Source:       crmcontracts.WorklistItemSource(item.Source),
		Category:     category,
		Level:        level,
		Consequence:  consequence,
		Kind:         item.Kind,
		Title:        item.Title,
		Detail:       item.Detail,
		NoticeOrigin: item.NoticeOrigin,
		CauseRef:     item.CauseRef,
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
		// The human behind the row, where the lane named one: a rep reads
		// whose row it is before choosing a verb (contacttouch.go).
		Contact: contactOf(item),
		// Whose page a meeting's brief opens on. Forwarded rather than derived
		// here: the lane already decided whether the reader may see anybody on
		// the meeting, and an absent value is that decision rather than a gap.
		WithContact: item.WithContact,
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
		// Carried back to the contact who APPROVED it, by a lane bound to them.
		ownerRef: ownedByWhoeverIsReading(),
	}
}

// classifyIntroduction: a colleague is waiting on this reader to answer.
//
// levelBlocking, which is "a decision that holds up customer work", because
// that is precisely what it is: a rep's deal is stopped until this colleague
// says yes, no, or ask somebody else. It is a DECISION rather than system news
// — a contact must choose, and only this contact can.
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
// without a date stays actionable without claiming an invented deadline.
func classifyTask(item crmcontracts.AttentionItem, asOf time.Time) ranked {
	level := levelAgreed
	// A due customer obligation needs attention even if its deal is small.
	// Prospecting follow-ups retain their dates without claiming an external
	// response deadline; that clock belongs to the lead-response lane.
	if item.DueAt != nil && (overdueAt(item.DueAt, asOf) || (item.DueGroup != nil && *item.DueGroup == crmcontracts.AttentionItemDueGroupToday)) &&
		(item.Subject == nil || item.Subject.Type != subjectLead) {
		level = levelPromise
	}
	row := base(item, level, "tasks", "task_slips")
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

func waitingSubject(waiting WaitingCustomer) *crmcontracts.AttentionSubject {
	switch {
	case !waiting.DealID.IsZero():
		return subjectOf(subjectDeal, waiting.DealID)
	case !waiting.ContactID.IsZero():
		return subjectOf("contact", waiting.ContactID)
	case !waiting.CompanyID.IsZero():
		return subjectOf("company", waiting.CompanyID)
	default:
		return nil
	}
}

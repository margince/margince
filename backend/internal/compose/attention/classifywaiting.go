// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package attention

// The waiting lane's classification: somebody wrote and nobody answered.
// Its own file because the rules of what demotes a wait — age, money,
// addressing, intent, a booked meeting — grew past what classify.go can
// carry beside the thirteen other sources.

import (
	"time"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
)

// valueDate is the WorklistValue kind for a moment.
const valueDate = "date"

// earlierRequests is the evidence a conversation card carries for the requests
// folded into it: how many, and when the first arrived. Nothing for a single
// message.
func earlierRequests(waiting WaitingCustomer) []crmcontracts.WorklistReason {
	if waiting.EarlierRequests == 0 {
		return nil
	}
	count, first := waiting.EarlierRequests, waiting.FirstAskedAt
	return []crmcontracts.WorklistReason{
		reason("earlier_requests", &crmcontracts.WorklistValue{Kind: "count", Count: &count}),
		reason("first_asked", &crmcontracts.WorklistValue{Kind: valueDate, Date: &first}),
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
// Old waits on open deals stay in the revenue recovery band.
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
	days := daysSince(waiting.Since, asOf)
	demoted := demotionsOf(waiting, days)
	row := crmcontracts.WorklistItem{
		Id:          waiting.ActivityID.String(),
		Source:      sourceWaiting,
		Category:    sourceWaiting,
		Level:       waitingLevel(waiting, demoted),
		Consequence: "buyer_waits",
		Because:     waitingBecause(waiting, days, demoted),
		Actions:     []crmcontracts.WorklistItemActions{},
	}
	if demoted.informational || waiting.ActionUnconfirmed || demoted.elsewhere {
		row.Consequence = "none"
	}
	// The subject travels because the row exists at all only for a reader the
	// content gate admitted: a message this reader may not read produces no
	// row, rather than a row with its words removed.
	if waiting.Subject != "" {
		row.Title = &waiting.Subject
	}
	// Present exactly when this wait is an email the reader may read. A client
	// branches on the field rather than on the kind word: the lane also carries
	// channel messages, and each keeps the plain title it had.
	row.EmailSummary = waiting.EmailSummary
	// The record the reply would be about, most specific first: the deal a
	// thread belongs to says more than the company it is filed under.
	row.Subject = waitingSubject(waiting)
	// The sender, whatever the subject: a thread filed under a deal is still a
	// message from a contact, and the reply goes to them.
	row.Contact = waitingContact(waiting)
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
	// A booked row neither claims nor is beaten by waiting time: its days go
	// to zero for the age comparator, so the explanation a neighbour row
	// publishes never credits an age the card itself no longer states.
	rankDays := days
	// asOf rather than Since for the terminal occurrence tie-break, which the
	// contract also explains as waiting time: a booked row neither claims an
	// age there nor outranks live work by one.
	rankedAt := waiting.Since
	if demoted.booked {
		rankDays = 0
		rankedAt = asOf
	}
	return ranked{
		item:        row,
		waitingDays: rankDays,
		waitingRank: orderingAge(rankDays),
		occurredAt:  rankedAt,
		// Whether this message is in a conversation, which decides what the
		// reader is offered: two of the three dispositions are keyed on the
		// thread and a threadless row can perform neither.
		threaded: waiting.Threaded,
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
		contact: waiting.ContactID,
	}
}

// waitingDemotions is which of the lane's demotions applied to one row —
// named fields rather than a row of booleans, so a call site reads as the
// judgement it states.
type waitingDemotions struct {
	stale, unproven, elsewhere, informational, booked bool
}

// demotionsOf judges one wait against every demotion the lane knows.
//
// unproven: nobody here has written on this thread and no money is on it.
// DEMOTED, never dropped — thread identity comes from the sender's reply
// headers, and a client that strips them gives every message its own thread,
// so hiding on it would lose a live customer with nothing on the page to say
// so. elsewhere: every header recipient names somebody other than this
// reader; the colleague has the same row on their own queue. informational:
// the classifier's explicit informs-us verdict. booked: a meeting with the
// sender was booked or moved since they wrote — the work is scheduled, and
// the lane's own rules wake the row as plain waiting when that meeting is
// canceled, a no-show, or over without being held.
func demotionsOf(waiting WaitingCustomer, days int) waitingDemotions {
	return waitingDemotions{
		stale:         days > waitingStaleDays,
		unproven:      !waiting.Engaged && !waiting.HasOpenDeal && !waiting.ConfirmedRequest,
		elsewhere:     waiting.AddressedElsewhere,
		informational: waiting.AsksNothing,
		booked:        waiting.MeetingBookedAt != nil,
	}
}

// waitingLevel is the band one wait claims once every demotion has spoken.
//
// Money outranks staleness and the unproven-thread demotion — an open deal is
// a stronger claim than any header the sender chose — and outranks nothing
// else: being addressed to a colleague asks WHOSE the work is, an
// informational verdict says no work is asked, and a booked meeting says the
// work is already scheduled. A deal's value changes none of those.
func waitingLevel(waiting WaitingCustomer, d waitingDemotions) int {
	level := levelWaiting
	if d.stale {
		level = levelRoutine
		if waiting.HasOpenDeal {
			level = levelMaterialRisk
		}
	}
	if d.unproven || d.elsewhere || d.informational || waiting.ActionUnconfirmed || d.booked {
		level = levelRoutine
	}
	return level
}

// waitingBecause is the evidence column of one waiting row, in the order the
// card states it: who moved last, then the count that replaces it when a
// meeting is booked, then every demotion that applied.
func waitingBecause(waiting WaitingCustomer, days int, d waitingDemotions) []crmcontracts.WorklistReason {
	because := []crmcontracts.WorklistReason{reason("buyer_wrote_last", nil)}
	if d.booked {
		because = append(because, reason("meeting_booked",
			&crmcontracts.WorklistValue{Kind: valueDate, Date: waiting.MeetingBookedAt}))
	} else {
		because = append(because, reason("waiting_days", daysValue(days)))
	}
	because = append(because, earlierRequests(waiting)...)
	if d.stale {
		because = append(because, reason("stale", nil))
	}
	if d.elsewhere {
		because = append(because, reason("addressed_elsewhere", nil))
	}
	if d.unproven {
		because = append(because, reason("no_reply_history", nil))
	}
	if d.informational {
		because = append(because, reason("asks_nothing", nil))
	}
	return because
}

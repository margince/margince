// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package dealstatus

import (
	"fmt"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/occurredday"
)

// currentRequest is the readable current request, whether or not we replied
// to it. The activities owner admits current requests before limiting the result.
func currentRequest(f facts) (crmcontracts.Activity, bool) {
	return readableRequest(f.currentRequests)
}

// unansweredInbound is the current request still owed our reply. One we
// replied to after it reads as ourReply instead.
func unansweredInbound(f facts) (crmcontracts.Activity, bool) {
	request, ok := currentRequest(f)
	if !ok || ourReplyOn(request) != nil {
		return crmcontracts.Activity{}, false
	}
	return request, true
}

// ourReplyOn is the activities owner's one rule for "we replied after the
// request", as it stamped the row: nil while their message is the newest.
func ourReplyOn(request crmcontracts.Activity) *crmcontracts.RequestReply {
	if request.EmailSummary == nil {
		return nil
	}
	return request.EmailSummary.OurReply
}

// ourReplyOnCard is the card's our_reply, read fresh for every reader: the
// reminder it names is gated by who reads it.
func ourReplyOnCard(f facts) *crmcontracts.RequestReply {
	if request, ok := currentRequest(f); ok {
		return ourReplyOn(request)
	}
	return nil
}

// repliedMove offers the reminder for completion once we replied. It opens the
// reminder rather than drafting a second reply, and the card draws its Done.
func repliedMove(said statusCopy, f facts, request crmcontracts.Activity, reply crmcontracts.RequestReply) crmcontracts.DealStatusCardMove {
	return ruleMove(ruleRequestReplied, said.repliedReason(f, reply),
		map[string]any{argActivityID: *reply.ReminderId},
		evidenceOf(request, fmt.Sprintf(said.Say(statusWords.Request), said.subjectOf(request))))
}

// repliedReason says who replied and when, then what is left. That is the ask
// the settlement pass quoted, or the reminder to mark done. A reader who may
// not change the reminder learns only that the request is open.
func (said statusCopy) repliedReason(f facts, reply crmcontracts.RequestReply) string {
	when := said.since(f.now, occurredday.Moment{At: reply.RepliedAt, DateOnly: occurredday.IsDay(reply.RepliedPrecision)}, f.loc)
	who := statusWords.RepliedTeam
	if reply.ByReader {
		who = statusWords.RepliedYou
	}
	rest := said.Say(statusWords.RepliedOpen)
	switch {
	case reply.StillAsked != nil:
		rest = fmt.Sprintf(said.Say(statusWords.RepliedAsked), *reply.StillAsked)
	case reply.ReminderId != nil:
		rest = said.Say(statusWords.RepliedMarkDone)
	}
	return fmt.Sprintf(said.Say(who), when) + " " + rest
}

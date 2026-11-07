// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contact360

import (
	"context"
	"fmt"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/shared/kernel/occurredday"
)

// awaitingReplyMoment: we wrote last, recently, and owe them nothing read out
// of a conversation. The next move is theirs.
//
// A commitment we made keeps its own card even after an unrelated mail of
// ours. Our reply on its thread is what asks whether it is done.
func (said momentCopy) awaitingReplyMoment(_ context.Context, now time.Time, page *crmcontracts.Contact360) (crmcontracts.ContactMoment, bool) {
	if page.LastOutboundAt == nil {
		return crmcontracts.ContactMoment{}, false
	}
	outbound := *page.LastOutboundAt
	if page.LastInboundAt != nil && !page.LastInboundAt.Before(outbound) {
		return crmcontracts.ContactMoment{}, false
	}
	if owesCommitment(page) {
		return crmcontracts.ContactMoment{}, false
	}
	waiting := occurredday.DaysSince(occurredday.Moment{At: outbound, DateOnly: occurredday.IsStatedDay(page.LastOutboundPrecision)}, now, said.zone)
	if waiting >= goneQuietAfterDays {
		return crmcontracts.ContactMoment{}, false
	}
	evidence := said.outboundEvidence(page, outbound, page.LastOutboundPrecision)
	return crmcontracts.ContactMoment{
		ClaimKey:            "moment:awaiting_reply",
		Rule:                crmcontracts.ContactMomentRuleAwaitingReply,
		RuleVersion:         new(ruleVersion),
		EvidenceFingerprint: fingerprintOf(evidence),
		Headline:            said.Say(momentWords.Awaiting),
		WhyNow:              said.awaitingWhyNow(waiting),
		Confidence:          crmcontracts.ContactMomentConfidenceObservedFact,
		Evidence:            evidence,
		FreshnessAt:         &outbound,
		RecommendedAction:   said.replyAction(said.Say(momentWords.FollowUp), "follow_up", page.LastOutboundActivityId),
	}, true
}

// owesCommitment reports whether an open promise read out of a conversation
// stands on the page, as a claim or as the task written for one. Other tasks
// are the rep's own reminders, such as chasing a reply, and keep the wait.
func owesCommitment(page *crmcontracts.Contact360) bool {
	if page.Claims != nil {
		for _, claim := range *page.Claims {
			if claim.Kind == crmcontracts.ConversationClaimKindCommitmentOurs &&
				claim.Status == crmcontracts.ConversationClaimStatusOpen && !claim.NeedsReview {
				return true
			}
		}
	}
	if page.NextSteps != nil {
		for _, task := range page.NextSteps.Data {
			if task.SourceSystem != nil && *task.SourceSystem == activities.CommitmentTaskSource {
				return true
			}
		}
	}
	return false
}

// awaitingWhyNow names how long ago our message went out and when the wait
// turns into a follow-up.
func (said momentCopy) awaitingWhyNow(waiting int) string {
	switch waiting {
	case 0:
		return fmt.Sprintf(said.Say(momentWords.AwaitingToday), goneQuietAfterDays)
	case 1:
		return fmt.Sprintf(said.Say(momentWords.AwaitingYesterday), goneQuietAfterDays)
	default:
		return fmt.Sprintf(said.Say(momentWords.AwaitingWhy), waiting, goneQuietAfterDays)
	}
}

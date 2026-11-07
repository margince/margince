// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contact360

import (
	"context"
	"fmt"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/occurredday"
)

// awaitingReplyMoment: we wrote last, recently, and promised nothing in or
// after that message. The next move is theirs.
//
// It sits above open_promise because the newest message outranks an older
// promise still on its clock. The commitments card still lists that promise.
// A late promise and a silence past goneQuietAfterDays rank above this rung.
func (said momentCopy) awaitingReplyMoment(_ context.Context, now time.Time, page *crmcontracts.Contact360) (crmcontracts.ContactMoment, bool) {
	if page.LastOutboundAt == nil {
		return crmcontracts.ContactMoment{}, false
	}
	outbound := *page.LastOutboundAt
	if page.LastInboundAt != nil && !page.LastInboundAt.Before(outbound) {
		return crmcontracts.ContactMoment{}, false
	}
	if promisedSince(page, outbound) {
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

// promisedSince reports whether an open commitment of ours was said in or
// after the message at `outbound`. Such a promise is the newest thing on the
// record, so we are the ones who owe the next move.
//
// Only claims carry the moment a promise was said. A task carries when it was
// filed, and a "chase them if silent" reminder is filed after the mail it waits on.
func promisedSince(page *crmcontracts.Contact360, outbound time.Time) bool {
	if page.Claims == nil {
		return false
	}
	for _, claim := range *page.Claims {
		if claim.Kind != crmcontracts.ConversationClaimKindCommitmentOurs ||
			claim.Status != crmcontracts.ConversationClaimStatusOpen || claim.NeedsReview {
			continue
		}
		if claim.OccurredAt != nil && !claim.OccurredAt.Before(outbound) {
			return true
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

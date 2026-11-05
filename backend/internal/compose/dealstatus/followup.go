// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package dealstatus

// The follow-up owed after a meeting on this deal. It is the reader's own
// meeting with nothing sent since, the same answer Home's meeting follow-up
// row gives, read through the same activities query.

import (
	"fmt"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/shared/kernel/occurredday"
)

// followUpOwed is the newest meeting on the deal that awaits a follow-up and
// has somebody on file to write to. A meeting with nobody on file has no
// draft to offer, so the card's later rungs speak for it.
func followUpOwed(f facts) (activities.AwaitingReply, bool) {
	for _, met := range f.followUps {
		if len(met.Recipients) > 0 {
			return met, true
		}
	}
	return activities.AwaitingReply{}, false
}

// followUpMove opens the composer to everyone the meeting was with. The
// recipients ride as contact links, so the queue re-checks each one on read.
func followUpMove(f facts, met activities.AwaitingReply) crmcontracts.DealStatusCardMove {
	said := statusCopyFor(f.lang)
	meetingID := openapi_types.UUID(met.ActivityID)
	links := make([]map[string]any, 0, len(met.Recipients))
	for _, contact := range met.Recipients {
		links = append(links, map[string]any{argEntityType: linkContact, argEntityID: openapi_types.UUID(contact)})
	}
	subject := met.Subject
	if subject == "" {
		subject = said.Say(statusWords.Meeting)
	}
	held := untimed(&meetingID, fmt.Sprintf(said.Say(statusWords.Met), subject))
	held.OccurredAt = &met.SentAt
	sent := occurredday.Moment{At: met.SentAt, DateOnly: occurredday.IsDay(met.Precision)}
	if sent.DateOnly {
		held.OccurredPrecision = crmcontracts.OccurredPrecisionOccurredPrecisionDay
	}
	return ruleMove(ruleMeetingFollowUp,
		fmt.Sprintf(said.Say(statusWords.FollowUp), said.since(f.now, sent, f.loc)),
		map[string]any{argActivityID: meetingID, argLinks: links}, held)
}

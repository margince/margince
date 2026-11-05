// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package attention

import (
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
)

func classifyDecay(item crmcontracts.AttentionItem, asOf time.Time) ranked {
	row := base(item, levelRoutine, categoryRelationships, "data_drifts")
	quiet := quietDaysOf(item)
	if quiet > 0 {
		row.Because = append(row.Because, reason("quiet_days", daysValue(quiet)))
	}
	row.Move = &crmcontracts.WorklistMove{Action: crmcontracts.WorklistMoveActionDraftEmail}
	return ranked{
		item: row, ownerRef: decayOwner(item.Relationship),
		waitingDays: quiet, waitingRank: orderingAge(quiet), occurredAt: occurredOf(item, asOf),
	}
}

// sourceDecay is the lane's own name on the wire.
//
// A constant rather than a literal at each match, because the failure mode of a
// typo here is silence: a suppressor comparing against a misspelt source simply
// never matches, so it suppresses nothing and every test that does not plant the
// duplicate row still passes.
const sourceDecay = crmcontracts.WorklistItemSource("relationship_decay")

// hasReason reports whether a row already states one fact.
func hasReason(item crmcontracts.WorklistItem, kind crmcontracts.WorklistReasonKind) bool {
	for _, because := range item.Because {
		if because.Kind == kind {
			return true
		}
	}
	return false
}

// categoryRelationships files quiet-relationship suggestions under their own
// badge and filter, apart from the product's reports about itself.
const categoryRelationships crmcontracts.WorklistItemCategory = "relationships"

// decayOwner is the live owner the lane read for the record: the only seat
// a quiet-relationship suggestion is for. No owner is unassigned, never the
// reader, so a colleague copied on the mail is not handed the follow-up.
func decayOwner(facts *crmcontracts.AttentionRelationshipFacts) ownerRef {
	if facts == nil {
		return unassigned()
	}
	return ownerFromAssignee(facts.OwnerId)
}

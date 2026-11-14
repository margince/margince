// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package attention

import (
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
)

// worklistKind is what the engine knows about one Worklist source. The values
// live in api/worklist-kinds.yaml; tools/gen-worklist writes worklistKinds
// from it, and every per-source answer below reads that table.
type worklistKind struct {
	category    crmcontracts.WorklistItemCategory
	destination crmcontracts.WorklistItemDestination
	// owner lists the owner answers a row of this kind may state. A row stating
	// another is refused by admitBySource.
	owner ownerAnswers
	// personal is a read bound to the acting member, which team and all
	// scopes cannot widen.
	personal    bool
	doneInPlace bool
}

// destinationNone is a fold's: the group carries its members' screen.
const destinationNone crmcontracts.WorklistItemDestination = ""

// Rowless side lanes are not Worklist kinds. A side lane that failed or came
// back at its bound is still reported under the category it feeds.
var sideLaneCategories = map[crmcontracts.WorklistItemSource]crmcontracts.WorklistItemCategory{
	sourceDealAdvice:  crmcontracts.WorklistItemCategoryDealsAtRisk,
	sourceCalendar:    crmcontracts.WorklistItemCategoryMeetings,
	sourceNextMeeting: crmcontracts.WorklistItemCategoryMeetings,
}

// categoryOfSource says which cut of the queue a source's rows land in, for a
// bounded or failed lane that left no row. A source the catalog does not name
// is reported as system, the answer that claims least.
func categoryOfSource(source crmcontracts.WorklistItemSource) crmcontracts.WorklistItemCategory {
	if kind, ok := worklistKinds[source]; ok {
		return kind.category
	}
	if category, ok := sideLaneCategories[source]; ok {
		return category
	}
	return crmcontracts.WorklistItemCategorySystem
}

// ownerAnswersOf is which owner answers a source's rows may state. An unknown
// source may state none.
func ownerAnswersOf(source crmcontracts.WorklistItemSource) (ownerAnswers, bool) {
	kind, ok := worklistKinds[source]
	return kind.owner, ok
}

// answersForTheActorOnly is whether a source's read is bound to the acting
// member. An unknown source reads as widening, which reach reports for side
// lanes that hold no rows of their own.
func answersForTheActorOnly(source crmcontracts.WorklistItemSource) bool {
	return worklistKinds[source].personal
}

// offersDone is whether a source's rows offer Done. dispositionsFor answers
// the same for a row, and always leads with Done where it offers any.
func offersDone(source crmcontracts.WorklistItemSource) bool {
	return worklistKinds[source].doneInPlace
}

// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package attention

import (
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
)

// The catalog's key set equals the contract's source enum both ways. A source
// with no entry would read every answer as its zero value. An entry for no
// source is a decision about nothing.
func TestEveryWorklistSourceHasExactlyOneKind(t *testing.T) {
	t.Parallel()
	declared := map[crmcontracts.WorklistItemSource]bool{}
	for _, source := range contractEnum(t, "WorklistItemSource") {
		declared[source] = true
		if _, ok := worklistKinds[source]; !ok {
			t.Errorf("%q is a WorklistItem.source with no entry in api/worklist-kinds.yaml", source)
		}
	}
	for source := range worklistKinds {
		if !declared[source] {
			t.Errorf("api/worklist-kinds.yaml declares %q, which crm.yaml does not offer as a WorklistItem.source", source)
		}
	}
}

func TestEveryKindAnswersInTheContractsVocabulary(t *testing.T) {
	t.Parallel()
	for source, kind := range worklistKinds {
		if !kind.category.Valid() {
			t.Errorf("%q is counted under %q, which crm.yaml does not declare as a category", source, kind.category)
		}
		folds := source == crmcontracts.WorklistItemSourceBatch
		if folds != (kind.destination == destinationNone) {
			t.Errorf("%q has destination %q: only a fold takes its members' screen", source, kind.destination)
		}
		if !folds && !kind.destination.Valid() {
			t.Errorf("%q is sent to %q, which crm.yaml does not declare as a destination", source, kind.destination)
		}
		if len(kind.owner) == 0 {
			t.Errorf("%q may state no owner answer, so none of its rows can reach a page", source)
		}
	}
}

// A side lane named as a kind would have two categories, and the catalog's
// would win without anyone deciding it.
func TestASideLaneIsNoWorklistKind(t *testing.T) {
	t.Parallel()
	for lane := range sideLaneCategories {
		if _, ok := worklistKinds[lane]; ok {
			t.Errorf("%q is both a side lane and a Worklist kind; keep it in api/worklist-kinds.yaml only", lane)
		}
		if !sideLaneCategories[lane].Valid() {
			t.Errorf("side lane %q is counted under %q, which crm.yaml does not declare", lane, sideLaneCategories[lane])
		}
	}
}

func TestASourceOutsideTheCatalogClaimsNothing(t *testing.T) {
	t.Parallel()
	const unknown = crmcontracts.WorklistItemSource("new_producer")
	if _, ok := ownerAnswersOf(unknown); ok {
		t.Error("an unknown source has owner answers")
	}
	if offersDone(unknown) || answersForTheActorOnly(unknown) {
		t.Error("an unknown source offers Done or claims a personal read")
	}
	if got := categoryOfSource(unknown); got != crmcontracts.WorklistItemCategorySystem {
		t.Errorf("an unknown source is counted under %q, want system", got)
	}
	if got := DestinationOfSource(unknown); got != destinationNone {
		t.Errorf("an unknown source is sent to %q", got)
	}
}

// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package attention

import (
	"context"
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestEveryWorklistSourceDeclaresWhoItsWorkIsFor(t *testing.T) {
	t.Parallel()
	known := map[crmcontracts.WorklistItemSource]bool{}
	for _, source := range everyReachSource(t) {
		known[source] = true
		if _, ok := sourceOwnerAnswers[source]; !ok {
			t.Errorf("%q has no admission policy, so none of its rows can reach a page", source)
		}
	}
	for source := range sourceOwnerAnswers {
		if !known[source] {
			t.Errorf("%q has a policy but the contract no longer offers it", source)
		}
	}
}

func TestEveryProducedRowGivesAnOwnerItsSourceAllows(t *testing.T) {
	t.Parallel()
	rows := classifyDay(dayOfEveryLane(), rankInstant, dayMoney{})
	if len(rows) == 0 {
		t.Fatal("the census classified nothing")
	}
	for _, row := range rows {
		if allowed := sourceOwnerAnswers[row.item.Source]; !allowed[row.ownerRef.kind] {
			t.Errorf("%q produced owner kind %d, which its policy does not allow", row.item.Source, row.ownerRef.kind)
		}
	}
}

func TestARowFromAnUnknownSourceCreatesNoWork(t *testing.T) {
	t.Parallel()
	known := classifyTask(crmcontracts.AttentionItem{Id: "task-1", Source: "task"}, rankInstant)
	unknown := known
	unknown.item.Source = "new_producer"
	unknown.ownerRef = ownedBy(ids.NewV7())

	kept := admitBySource(context.Background(), []ranked{unknown, known})
	if len(kept) != 1 || kept[0].item.Source != "task" {
		t.Fatalf("admitted = %+v, want only the row whose source has a policy", kept)
	}
}

func TestARowClaimingTheReaderForAnOwnerColumnSourceIsNobodys(t *testing.T) {
	t.Parallel()
	row := classifyDecay(lapsedItem(QuietRelationship{ContactID: ids.NewV7(), Name: "Ines"}), rankInstant)
	row.ownerRef = ownedByWhoeverIsReading()

	kept := admitBySource(context.Background(), []ranked{row})
	if len(kept) != 1 || kept[0].ownerRef.kind != ownerNobody {
		t.Fatalf("admitted = %+v, want the row kept and unassigned, never the reader's", kept)
	}
}

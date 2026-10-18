// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package attention

// Scope filters and cards resolve the same owner, including folded rows.

import (
	"testing"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestANamedOwnerIsOneTheScopeFiltersCanRead(t *testing.T) {
	t.Parallel()
	holder := ids.NewV7()
	rows := classifyDay(dayWhoseRowsNameAnOwner(holder), rankInstant, dayMoney{})
	rows = append(rows,
		classifyWaiting(WaitingCustomer{Since: rankInstant, OwnerID: holder}, rankInstant))
	named := 0
	for _, row := range rows {
		if row.ownerRef.kind != ownerNamed {
			continue
		}
		named++
		answers, readable := answersTo(row)
		if !readable {
			t.Errorf("a %q row names %v to the reader and answersTo sees nobody, so the row "+
				"prints an owner but cannot scope the row to that owner",
				row.item.Source, row.ownerRef.user)
			continue
		}
		if answers != row.ownerRef.user {
			t.Errorf("a %q row names %v to the reader and %v to the scope filters, so the card "+
				"and the queue it sits on disagree about who owes it",
				row.item.Source, row.ownerRef.user, answers)
		}
	}
	// Without this the census passes over a day whose producers all answered
	// `unassigned` or `whoever is reading` — every arm above skipped, nothing
	// compared, and PASS. The fixture must actually reach the paired answer.
	if named < 2 {
		t.Fatalf("only %d row named a contact, so this census compared almost nothing: "+
			"give dayWhoseRowsNameAnOwner a row for each producer that calls ownedBy", named)
	}
}

// dayWhoseRowsNameAnOwner is dayOfEveryLane with a contact on the rows that can
// carry one, so the pairing above is exercised rather than skipped.
func dayWhoseRowsNameAnOwner(holder ids.UUID) crmcontracts.Attention {
	day := dayOfEveryLane()
	assignee := openapi_types.UUID(holder)
	day.Planned = []crmcontracts.AttentionItem{
		item("task", "task", func(row *crmcontracts.AttentionItem) { row.AssigneeId = &assignee }),
	}
	return day
}

func TestAFoldedRowNamesTheSameOwnerForScopeAndDisplay(t *testing.T) {
	t.Parallel()
	holder := ids.NewV7()
	members := []ranked{{ownerRef: ownedBy(holder)}, {ownerRef: ownedBy(holder)}}
	group := ownerOfTheGroup(members)
	if group.kind != ownerNamed || group.user != holder {
		t.Fatalf("a pile agreeing on one owner says %v, so this test no longer reaches "+
			"the shape it was written for", group)
	}
	folded := ranked{ownerRef: group}
	if got, readable := answersTo(folded); !readable || got != holder {
		t.Fatalf("folded owner = %v, readable %v; want %v", got, readable, holder)
	}
}

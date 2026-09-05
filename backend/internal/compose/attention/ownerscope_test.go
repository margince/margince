// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package attention

// One row, two owner answers, and nothing holding them together.
//
// A ranked row states its owner twice: `ownerRef` answers the CLIENT, and
// `owner` answers the SCOPE FILTERS through answersTo. The split is deliberate
// — a zero `owner` is a row the filters simply keep, where a client told
// `unassigned` would read a confident claim nobody owes it — and only `owner`
// decides whose Mine queue a row lands on.
//
// So a producer that names a person in `ownerRef` and leaves `owner` zero puts
// a row on nobody's queue while the card beside it prints that person's name.
// The existing census (TestEveryProducerStatesAnOwner) cannot see that: it reads
// `ownerRef` alone, which is the half that would still be right.
//
// Today the three producers that name a person set both. This test is what
// makes that stay true.

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
		classifyWaiting(WaitingCustomer{Since: rankInstant, OwnerID: holder}, rankInstant),
		classifyLead(OwedLead{OwnerID: holder}, rankInstant))
	named := 0
	for _, row := range rows {
		if row.ownerRef.kind != ownerNamed {
			continue
		}
		named++
		answers, readable := answersTo(row)
		if !readable {
			t.Errorf("a %q row names %v to the reader and answersTo sees nobody, so the row "+
				"prints an owner and lands on no Mine queue: set `owner` in its classifier too",
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
	if named < 3 {
		t.Fatalf("only %d row named a person, so this census compared almost nothing: "+
			"give dayWhoseRowsNameAnOwner a row for each producer that calls ownedBy", named)
	}
}

// dayWhoseRowsNameAnOwner is dayOfEveryLane with a person on the rows that can
// carry one, so the pairing above is exercised rather than skipped.
func dayWhoseRowsNameAnOwner(holder ids.UUID) crmcontracts.Attention {
	day := dayOfEveryLane()
	assignee := openapi_types.UUID(holder)
	day.Planned = []crmcontracts.AttentionItem{
		item("task", "task", func(row *crmcontracts.AttentionItem) { row.AssigneeId = &assignee }),
	}
	return day
}

// A folded row can name a person the scope filters will never see.
//
// ownerOfTheGroup speaks where the members agree, and a synthesized row has no
// record of its own to take `owner` from — so a group whose members all name
// one person carries ownerNamed with a zero `owner`. That is the mis-scoping
// the census above describes, reached directly because no producer that folds
// can currently produce it: classifyDecision, the only classifier the fold
// consumes, answers unassigned() for every row and ignores AssigneeId, by a
// deliberate rule of its own.
//
// So this is the shape waiting rather than a defect today, and testing it here
// rather than through a fixture keeps the claim honest: the census above cannot
// reach this, and pretending otherwise with a hand-forced pile would assert a
// path through classifyDecision that does not exist.
func TestAFoldedRowNamingAPersonWouldNotBeScopedToThem(t *testing.T) {
	t.Parallel()
	holder := ids.NewV7()
	members := []ranked{{ownerRef: ownedBy(holder)}, {ownerRef: ownedBy(holder)}}
	group := ownerOfTheGroup(members)
	if group.kind != ownerNamed || group.user != holder {
		t.Fatalf("a pile agreeing on one owner says %v, so this test no longer reaches "+
			"the shape it was written for", group)
	}
	// The gap itself: the group names a person and carries no `owner` for
	// answersTo to read. If a foldable producer ever names one, this is the line
	// that has to change with it.
	folded := ranked{ownerRef: group}
	if _, readable := answersTo(folded); readable {
		t.Error("a folded row now answers the scope filters — fold its owner into the " +
			"census above rather than leaving this test to describe a gap that closed")
	}
}

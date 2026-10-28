// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package attention

import (
	"testing"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func quietRow(owner ids.UUID) ranked {
	return classifyDecay(lapsedItem(QuietRelationship{
		ContactID: ids.NewV7(), Name: "Ines Sommer", OwnerID: owner, QuietDays: 60, LastAt: rankInstant,
	}), rankInstant)
}

func TestAQuietRelationshipIsTheContactOwnersAndNobodyElses(t *testing.T) {
	t.Parallel()
	owner := ids.NewV7()
	reader := ids.NewV7()
	wire := ownerOnTheWire(quietRow(owner), reader)
	if wire == nil || wire.Kind != crmcontracts.WorklistOwnerKindWorklistOwnerUser || wire.Id == nil ||
		*wire.Id != openapi_types.UUID(owner) {
		t.Fatalf("owner = %+v, want the contact's owner %s, not the reader %s", wire, owner, reader)
	}
}

func TestAQuietRelationshipWithNoLiveOwnerIsNobodysPersonalWork(t *testing.T) {
	t.Parallel()
	wire := ownerOnTheWire(quietRow(ids.UUID{}), ids.NewV7())
	if wire == nil || wire.Kind != crmcontracts.WorklistOwnerKindWorklistOwnerUnassigned {
		t.Fatalf("owner = %+v, want unassigned: no fallback to the reader", wire)
	}
}

func TestAQuietRelationshipHasItsOwnCategory(t *testing.T) {
	t.Parallel()
	row := quietRow(ids.NewV7())
	if row.item.Category != categoryRelationships {
		t.Errorf("category = %q, want %q", row.item.Category, categoryRelationships)
	}
	if got := categoryOfSource(sourceDecay); got != categoryRelationships {
		t.Errorf("the source's category = %q, want %q so counts and filter agree", got, categoryRelationships)
	}
}

// A quiet scan that spent its budget says so even when no row survived the
// derivation. An empty lane is then a lower bound, not a clear one.
func TestAnExhaustedQuietScanWithNoSurvivorsIsNotComplete(t *testing.T) {
	t.Parallel()
	more := true
	day := crmcontracts.Attention{AsOf: rankInstant, RelationshipDecay: &[]crmcontracts.AttentionItem{}, RelationshipDecayMoreAvailable: &more}
	out := (&Service{}).worklistFrom(t.Context(), day, scopeAll, "", 25, waitingRead{}, worklistCursor{}, nil)
	if out.OptionalRelationshipsMoreAvailable == nil || !*out.OptionalRelationshipsMoreAvailable {
		t.Fatalf("optional scan hid its bound: %+v", out)
	}
	for _, count := range out.Counts {
		if string(count.Category) == string(categoryRelationships) && count.Considered != 0 {
			t.Fatalf("optional relationship became responsible work: %+v", count)
		}
	}
}

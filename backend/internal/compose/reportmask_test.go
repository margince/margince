// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// What the report engine must exclude is what the caller's masks WITHHOLD on
// the report's entity, which is not the same set as what those masks are
// configured ON: a mask reaches the fields of every record that republishes its
// fact, and the report over that other record is where an aggregate would hand
// the value back in a total.

import (
	"context"
	"testing"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
)

// marginMaskedActor carries one mask, configured on the partner and reaching
// the commission entry that freezes the tier at accrual.
func marginMaskedActor() context.Context {
	return principal.WithActor(context.Background(), principal.Principal{
		Type: principal.PrincipalHuman,
		Permissions: principal.Permissions{
			RowScope: principal.RowScopeTeam,
			Objects:  map[string]principal.ObjectGrant{"commission": {Read: true}},
			FieldMasks: []principal.FieldMask{
				{Object: "partner", Field: "margin_tier", Condition: principal.MaskAlways},
			},
		},
	})
}

// The engine's rule stated as one implication: whatever the closure withholds
// on the entity, the report excludes. Selecting the caller's masks by the
// object they name breaks it for a mask configured elsewhere — the report finds
// none of its own, emits no clause, and aggregates the column in the open.
func TestAReportExcludesTheRowsAMaskReachesFromAnotherObject(t *testing.T) {
	t.Parallel()
	ctx := marginMaskedActor()
	spec := reportSpec{entity: datasource.EntityType("commission"), table: "commission_entry"}

	actor, ok := principal.Actor(ctx)
	if !ok {
		t.Fatal("the fixture carries no actor")
	}
	withheld := auth.MaskedFields(actor, string(spec.entity), false)
	if len(withheld) == 0 {
		t.Fatalf("nothing is withheld on %s, so this test would pass over the defect it exists for", spec.entity)
	}

	clauses, masked, err := maskExclusionClauses(ctx, spec, func(any) int { return 1 })
	if err != nil {
		t.Fatalf("rendering the report's mask clauses: %v", err)
	}
	if !masked || len(clauses) == 0 {
		t.Errorf("a report over %s emitted no exclusion clause while %v is withheld on it: "+
			"the aggregate would hand the value back as a total", spec.entity, withheld)
	}
}

// The other direction: a caller nothing is withheld from aggregates every row,
// or every report would report a governed subset of itself.
func TestAReportExcludesNothingFromAnUnmaskedCaller(t *testing.T) {
	t.Parallel()
	ctx := principal.WithActor(context.Background(), principal.Principal{
		Type:        principal.PrincipalHuman,
		Permissions: principal.Permissions{RowScope: principal.RowScopeTeam},
	})
	spec := reportSpec{entity: datasource.EntityDeal, table: "deal"}

	clauses, masked, err := maskExclusionClauses(ctx, spec, func(any) int { return 1 })
	if err != nil {
		t.Fatalf("rendering the report's mask clauses: %v", err)
	}
	if masked || len(clauses) != 0 {
		t.Errorf("an unmasked caller's report excluded rows: %v", clauses)
	}
}

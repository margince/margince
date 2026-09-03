// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The derived schema must narrow with the caller, and must say nothing about
// what it removed.

import (
	"context"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/compose/analyticsquery"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// seatWith builds a caller holding exactly the named objects.
func seatWith(objects ...string) context.Context {
	grants := map[string]principal.ObjectGrant{}
	for _, object := range objects {
		grants[object] = principal.ObjectGrant{Read: true}
	}
	ctx := principal.WithWorkspaceID(context.Background(), ids.NewV7())
	return principal.WithActor(ctx, principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:x", UserID: ids.NewV7(),
		Permissions: principal.Permissions{Objects: grants, RowScope: principal.RowScopeAll},
	})
}

func TestTheDerivedSchemaNarrowsWithTheCallersGrants(t *testing.T) {
	t.Parallel()
	wide := AnalyticsSchemaFor(seatWith("deal", "activity", "person", "organization", "project", "partner"))
	narrow := AnalyticsSchemaFor(seatWith("deal"))

	if len(wide.Entities) == 0 {
		t.Fatal("a seat holding every grant was handed an empty vocabulary")
	}
	// The narrower seat asks about no more than the wider one. Asserted as a
	// SUBSET rather than as a count: a count would pass if the narrow seat
	// gained one field while losing two.
	for name, entity := range narrow.Entities {
		wideEntity, ok := wide.Entities[name]
		if !ok {
			t.Errorf("the narrower seat can ask about %q and the wider one cannot", name)
			continue
		}
		for field := range entity.Fields {
			if _, ok := wideEntity.Fields[field]; !ok {
				t.Errorf("the narrower seat can name %s.%s and the wider one cannot", name, field)
			}
		}
	}
}

func TestAFieldACallerMayNotReadIsAbsentRatherThanRefused(t *testing.T) {
	t.Parallel()
	// The disclosure this prevents: "you may not read that" says the column
	// exists. Absent means the refusal is "no such field", which says nothing.
	narrow := AnalyticsSchemaFor(seatWith("deal"))
	wide := AnalyticsSchemaFor(seatWith("deal", "activity", "person", "organization", "project", "partner"))

	removed := 0
	for name, wideEntity := range wide.Entities {
		narrowEntity, ok := narrow.Entities[name]
		if !ok {
			removed += len(wideEntity.Fields)
			continue
		}
		for field := range wideEntity.Fields {
			if _, ok := narrowEntity.Fields[field]; !ok {
				removed++
			}
		}
	}
	// The floor for this test: if the two seats saw the same vocabulary, every
	// assertion above would pass over nothing.
	if removed == 0 {
		t.Fatal("a deal-only seat sees the same vocabulary as one holding every grant — the narrowing reached nothing")
	}

	// And a query naming a removed field is refused as absent.
	for name, wideEntity := range wide.Entities {
		narrowEntity, ok := narrow.Entities[name]
		if !ok {
			continue
		}
		for field, def := range wideEntity.Fields {
			if _, ok := narrowEntity.Fields[field]; ok {
				continue
			}
			q := analyticsquery.Query{
				Entity:   name,
				Measures: []analyticsquery.Measure{{Fn: analyticsquery.CountAll}},
				Filters: []analyticsquery.Filter{
					{Field: field, Op: analyticsquery.OpEq, Value: "x"},
				},
			}
			err := q.Validate(narrow)
			if err == nil {
				t.Errorf("%s.%s is withheld from this seat and a query naming it compiled", name, field)
				continue
			}
			if strings.Contains(err.Error(), "permission") || strings.Contains(err.Error(), "may not") {
				t.Errorf("the refusal for %s.%s says the field exists: %v", name, field, err)
			}
			_ = def
			return
		}
	}
}

func TestTheSchemaVersionMovesWhenTheVocabularyDoes(t *testing.T) {
	t.Parallel()
	// A compiled plan carries this version and is refused once it moves. If it
	// did NOT move when a seat lost a grant, that seat's outstanding plans
	// would go on running against a vocabulary they no longer hold.
	wide := AnalyticsSchemaFor(seatWith("deal", "activity", "person", "organization", "project", "partner"))
	narrow := AnalyticsSchemaFor(seatWith("deal"))
	if wide.Version == narrow.Version {
		t.Error("two seats with different vocabularies share a schema version, so a plan survives a grant being removed")
	}
	// And it is STABLE for one vocabulary: a version that changed per call
	// would refuse every plan immediately.
	if AnalyticsSchemaFor(seatWith("deal")).Version != narrow.Version {
		t.Error("the same vocabulary hashed to two versions, so every plan is refused")
	}
}

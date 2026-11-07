// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The derived schema must narrow with the caller, and must say nothing about
// what it removed.

import (
	"context"
	"maps"
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
	wide := AnalyticsSchemaFor(seatWith("deal", "activity", "contact", "company", "project", "partner"))
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
	wide := AnalyticsSchemaFor(seatWith("deal", "activity", "contact", "company", "project", "partner"))

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
	wide := AnalyticsSchemaFor(seatWith("deal", "activity", "contact", "company", "project", "partner"))
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

// A model reads the currency rule from the vocabulary before it asks, and a
// saved answer from before a measure became per-row money does not compare as
// the same vocabulary.
func TestTheVocabularyPublishesWhichMeasuresAreInEachRowsCurrency(t *testing.T) {
	t.Parallel()
	schema := AnalyticsSchemaFor(seatWith("deal"))
	described := DescribeAnalyticsSchema(schema)
	marked := 0
	for name, entity := range schema.Entities {
		for field, def := range entity.Fields {
			if def.Currency == nil {
				continue
			}
			marked++
			if !strings.Contains(described, field+" is in each row's own currency: group by "+def.Currency.By) {
				t.Errorf("%s.%s is per-row money and the vocabulary does not say so", name, field)
			}
			stripped := maps.Clone(schema.Entities)
			fields := maps.Clone(entity.Fields)
			def.Currency = nil
			fields[field] = def
			entity.Fields = fields
			stripped[name] = entity
			if schemaVersion(stripped) == schema.Version {
				t.Errorf("dropping the currency basis of %s.%s left the schema version unchanged", name, field)
			}
		}
	}
	if marked == 0 {
		t.Fatal("no deal measure is marked as per-row money, so this test swept nothing")
	}
}

// A population whose base entity the seat may not read is absent from the
// derived schema — not narrowed, not refused by name: absent. The vocabulary
// document and the unknown-name refusal both serve the schema, so this is
// what keeps them from naming populations ahead of the object gate.
func TestAPopulationWhoseEntityTheSeatCannotReadIsAbsent(t *testing.T) {
	t.Parallel()
	sansDeal := AnalyticsSchemaFor(seatWith("activity", "contact", "company", "project", "partner"))
	wide := AnalyticsSchemaFor(seatWith("deal", "activity", "contact", "company", "project", "partner"))

	// Judged against the WHOLE catalog, so a new deal population cannot land
	// outside the assertion.
	dealPopulations := 0
	for key, spec := range prebuiltReports {
		if string(spec.entity) != "deal" {
			continue
		}
		dealPopulations++
		if _, ok := sansDeal.Entities[key]; ok {
			t.Errorf("%q reads deals and is offered to a seat without deal read", key)
		}
		// The positive control: the same population reaches a seat holding
		// the grant, or the absence above proves nothing.
		if _, ok := wide.Entities[key]; !ok {
			t.Errorf("%q is absent from a fully-granted seat too, so the withholding above is vacuous", key)
		}
	}
	if dealPopulations == 0 {
		t.Fatal("no prebuilt report reads deals, so this test swept nothing")
	}
}

// A row's own id, name and key answer a group of one, below any floor, and a
// filter on one leaves a population of one. The vocabulary names none of them.
// A row's own timestamp can still bound a range, so it narrows and never groups.
func TestTheVocabularyOffersNoFieldThatCanOnlyBeWithheld(t *testing.T) {
	t.Parallel()
	schema := AnalyticsSchemaFor(seatWith("deal", "activity", "contact", "company", "project", "partner"))
	for _, name := range []string{"project-commitments", "projects-gone-quiet"} {
		entity, ok := schema.Entities[name]
		if !ok {
			t.Fatalf("%s is absent from a vocabulary that holds every grant", name)
		}
		for _, field := range []string{fieldProjectID, fieldName, fieldKey} {
			if got, named := entity.Fields[field]; named {
				t.Errorf("%s names %+v, which can only be withheld", name, got)
			}
		}
		for _, field := range []string{fieldPhase, fieldOwnerID, fieldCompanyID} {
			if got := entity.Fields[field]; got.Kind != analyticsquery.KindDimension {
				t.Errorf("%s.%s = %+v, want it still offered as a grouping", name, field, got)
			}
		}
	}
	quiet := schema.Entities["projects-gone-quiet"]
	for _, field := range []string{fieldLastActivityAt, fieldQuietSince} {
		if got := quiet.Fields[field]; got.Kind != analyticsquery.KindFilter {
			t.Errorf("projects-gone-quiet.%s = %+v, want a field to narrow by and not to group by", field, got)
		}
	}
	for name, entity := range schema.Entities {
		for field, def := range entity.Fields {
			if def.Expr == colRowID {
				t.Errorf("%s.%s names the row's own id", name, field)
			}
		}
	}
}

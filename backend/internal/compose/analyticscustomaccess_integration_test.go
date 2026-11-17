// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/analyticsquery"
	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// repDeals adds deals owned by Rep1, all in one territory, to the fixture's.
func repDeals(t *testing.T, e *integration.Env, fixture territory, territory string, n int) {
	t.Helper()
	owner := ids.From[ids.UserKind](e.Rep1)
	for i := range n {
		if _, err := fixture.deals.CreateDeal(e.Admin(), deals.CreateDealInput{
			Name: fmt.Sprintf("Rep deal %d", i), PipelineID: fixture.pipeline, StageID: fixture.open,
			OwnerID: &owner, CustomFields: map[string]any{*fixture.field.ColumnName: territory},
		}); err != nil {
			t.Fatal(err)
		}
	}
}

// ownDealsRep is a rep's seat with an own row scope.
func ownDealsRep(e *integration.Env) context.Context {
	return e.As(e.Rep1, nil, principal.Permissions{
		Objects: map[string]principal.ObjectGrant{
			"deal": {Read: true}, "installation_settings": {Read: true},
		},
		RowScope: principal.RowScopeOwn,
	})
}

// A custom field adds no row scope of its own and takes none away. Grouped by
// territory, a rep counts the same deals as grouped by a built-in field.
func TestACustomGroupingCoversTheSamePopulationAsABuiltInOne(t *testing.T) {
	t.Parallel()
	e := integration.Setup(t)
	fixture := territoryDeals(t, e)
	optIn(t, e, fixture.catalog, fixture.field)
	repDeals(t, e, fixture, "EMEA", 2)
	column := *fixture.field.ColumnName

	ctx := ownDealsRep(e)
	byTerritory, err := dealsByColumn(ctx, e, column)
	if err != nil {
		t.Fatalf("the rep's grouping by territory was refused: %v", err)
	}
	byStatus, err := dealsByColumn(ctx, e, "status")
	if err != nil {
		t.Fatalf("the rep's grouping by status was refused: %v", err)
	}
	if got, want := totalCount(t, byTerritory), totalCount(t, byStatus); got != want || got != 5 {
		t.Errorf("by territory the rep counts %d deals, by status %d, want the same 5", got, want)
	}
}

// The records behind a territory's count are the ones the count was taken
// over, no more and no fewer.
func TestTheRecordsBehindACustomGroupAreTheOnesItCounts(t *testing.T) {
	t.Parallel()
	e := integration.Setup(t)
	fixture := territoryDeals(t, e)
	optIn(t, e, fixture.catalog, fixture.field)
	repDeals(t, e, fixture, "EMEA", 2)
	ctx := ownDealsRep(e)

	var explained AnalyticsExplanation
	if err := database.WithWorkspaceTx(ctx, e.Pool, func(tx pgx.Tx) error {
		var err error
		explained, err = ExplainAnalyticsCell(ctx, tx, analyticsquery.Explain{
			Query: analyticsquery.Query{
				Entity: "deals-by-stage", GroupBy: []string{*fixture.field.ColumnName},
				Measures: []analyticsquery.Measure{{Fn: analyticsquery.CountAll}},
			},
			Group: []any{"EMEA"},
		}, analyticsquery.Floor(1))
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if len(explained.Rows) != 4 {
		t.Errorf("the EMEA cell opens %d records to the rep, want the 4 deals it counts", len(explained.Rows))
	}
}

func totalCount(t *testing.T, answer AnalyticsAnswer) int {
	t.Helper()
	total := 0
	for _, row := range answer.Rows {
		total += countOf(t, row)
	}
	return total
}

// A filter on a custom field that leaves out fewer deals than the floor is
// refused. Set beside the unfiltered answer, it would describe those deals.
func TestACustomFieldFilterThatSeparatesOutTooFewDealsIsRefused(t *testing.T) {
	t.Parallel()
	e := integration.Setup(t)
	fixture := territoryDeals(t, e)
	optIn(t, e, fixture.catalog, fixture.field)
	repDeals(t, e, fixture, "EMEA", 4)
	ctx := e.Admin()

	err := database.WithWorkspaceTx(ctx, e.Pool, func(tx pgx.Tx) error {
		_, err := RunAnalyticsQuery(ctx, tx, analyticsquery.Query{
			Entity:   "deals-by-stage",
			Measures: []analyticsquery.Measure{{Fn: analyticsquery.CountAll}},
			Filters: []analyticsquery.Filter{
				{Field: *fixture.field.ColumnName, Op: analyticsquery.OpNe, Value: "APAC"},
			},
		}, analyticsquery.DefaultFloor)
		return err
	})
	refusal, ok := errors.AsType[*analyticsquery.RefusalError](err)
	if !ok || refusal.Kind != analyticsquery.RefusalPrivacy {
		t.Fatalf("a filter leaving out the one APAC deal answered %v, want a privacy refusal", err)
	}
}

// A grant that only narrows the analytics vocabulary is asked unrecorded, so
// tools/list keeps offering the schema to a role without it. Naming the field
// outright still asks it recorded, and refuses.
func TestAnOptionalAnalyticsGrantNarrowsTheSchemaWithoutHidingTheTool(t *testing.T) {
	t.Parallel()
	specs := map[string]reportSpec{"projects-by-phase": projectsByPhaseSpec()}
	ctx, recorded := auth.RecordChecksForTest(seatWith("project", "deal"))

	schema := analyticsSchemaFrom(ctx, specs)
	if _, named := schema.Entities["projects-by-phase"].Fields[fieldCompanyID]; named {
		t.Errorf("a role without company and relationship reads can name %s", fieldCompanyID)
	}
	for _, optional := range []string{"company.read", "relationship.read"} {
		if slices.Contains(recorded().Checks, optional) {
			t.Errorf("deriving the schema recorded %s, which would hide the tool from a role without it", optional)
		}
	}

	err := requireVocabularyGrants(ctx, projectsByPhaseSpec(), []string{fieldCompanyID})
	if !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Fatalf("naming %s without the grants answered %v, want a refusal", fieldCompanyID, err)
	}
	if !slices.Contains(recorded().Checks, "company.read") {
		t.Errorf("the refusing check was not recorded: %v", recorded().Checks)
	}

	// One grant held and the other not: each is asked unrecorded on its own.
	relationOnly, recordedOne := auth.RecordChecksForTest(seatWith("project", "deal", "company"))
	analyticsSchemaFrom(relationOnly, specs)
	if slices.Contains(recordedOne().Checks, "relationship.read") {
		t.Error("deriving the schema without relationship.read recorded it")
	}

	wide := analyticsSchemaFrom(seatWith("project", "deal", "company", "relationship"), specs)
	if _, named := wide.Entities["projects-by-phase"].Fields[fieldCompanyID]; !named {
		t.Errorf("a role with both grants cannot name %s", fieldCompanyID)
	}
}

// A population a role cannot read narrows the schema without hiding the tool.
// One any-of check is recorded, so a role that reads no population at all is
// still not offered it.
func TestAnUnreadablePopulationNarrowsTheSchemaWithoutHidingTheTool(t *testing.T) {
	t.Parallel()
	ctx, recorded := auth.RecordChecksForTest(seatWith("deal"))
	schema := AnalyticsSchemaFor(ctx)
	if len(schema.Entities) == 0 {
		t.Fatal("a role reading deals was handed no population")
	}
	for key := range schema.Entities {
		if entity := prebuiltReports[key].entity; entity != "deal" {
			t.Errorf("a role reading only deals was handed %s, a %s population", key, entity)
		}
	}
	anyOf := ""
	for _, check := range recorded().Checks {
		if strings.Contains(check, ",") {
			anyOf = check
			continue
		}
		if check == "activity.read" || check == "lead.read" || check == "project.read" {
			t.Errorf("deriving the schema recorded %s alone, which would hide the tool from a role without it", check)
		}
	}
	for _, check := range analyticsPopulationChecks(prebuiltReports) {
		if !strings.Contains(anyOf, check.String()) {
			t.Errorf("the recorded population check %q leaves out %s", anyOf, check)
		}
	}

	none, recordedNone := auth.RecordChecksForTest(seatWith())
	if got := AnalyticsSchemaFor(none); len(got.Entities) != 0 {
		t.Errorf("a role reading no population was handed %d", len(got.Entities))
	}
	if !slices.Contains(recordedNone().Refused, anyOf) {
		t.Errorf("a role reading no population was not refused the any-of check: %v", recordedNone().Refused)
	}
}

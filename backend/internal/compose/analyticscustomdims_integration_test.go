// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/analyticsquery"
	"github.com/margince/margince/backend/internal/compose/installseam"
	"github.com/margince/margince/backend/internal/compose/integration"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/customfields"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// territoryField creates a Deal picklist through the real catalog writer and
// seeds deals carrying its values through the real deal writer.
func territoryField(t *testing.T, e *integration.Env) (*customfields.Service, crmcontracts.CustomField) {
	t.Helper()
	catalog := customfields.NewService(e.Pool, integration.SchemaPool(t))
	admin := e.As(e.Rep1, nil, integration.CustomFieldAdminPerms)
	field, err := catalog.Create(admin, customfields.FieldSpec{
		Object: "deal", Label: "Territory", Type: customfields.TypePicklist,
		Options: []string{"EMEA", "APAC"}, Source: "manual",
	})
	if err != nil {
		t.Fatalf("create the territory field: %v", err)
	}
	pipeline, open, _ := integration.DealFixture(t, e)
	store := deals.NewStore(e.DB(), installseam.Deals()).WithFieldCatalog(catalog)
	for i, territory := range []string{"EMEA", "EMEA", "APAC"} {
		if _, err := store.CreateDeal(e.Admin(), deals.CreateDealInput{
			Name: "Territory deal " + string(rune('A'+i)), PipelineID: pipeline, StageID: open,
			CustomFields: map[string]any{*field.ColumnName: territory},
		}); err != nil {
			t.Fatalf("seed deal %d: %v", i, err)
		}
	}
	return catalog, field
}

func optIn(t *testing.T, e *integration.Env, catalog *customfields.Service, field crmcontracts.CustomField) {
	t.Helper()
	yes := true
	if _, err := catalog.Update(e.As(e.Rep1, nil, integration.CustomFieldAdminPerms),
		ids.UUID(field.Id), customfields.FieldPatch{Reportable: &yes}, nil); err != nil {
		t.Fatalf("opt the field into reporting: %v", err)
	}
}

func dealsByColumn(ctx context.Context, e *integration.Env, column string) (AnalyticsAnswer, error) {
	var answer AnalyticsAnswer
	err := database.WithWorkspaceTx(ctx, e.Pool, func(tx pgx.Tx) error {
		var err error
		answer, err = RunAnalyticsQuery(ctx, tx, analyticsquery.Query{
			Entity:   "deals-by-stage",
			GroupBy:  []string{column},
			Measures: []analyticsquery.Measure{{Fn: analyticsquery.CountAll}},
		}, analyticsquery.Floor(1))
		return err
	})
	return answer, err
}

// A field an administrator has not opted in is not a dimension: grouping by
// it is refused by name, the same refusal any unknown field gets.
func TestACustomFieldIsNotADimensionUntilAnAdminOptsItIn(t *testing.T) {
	e := integration.Setup(t)
	_, field := territoryField(t, e)

	_, err := dealsByColumn(e.Admin(), e, *field.ColumnName)
	var refusal *analyticsquery.RefusalError
	if !errors.As(err, &refusal) {
		t.Fatalf("grouping by a field nobody opted in answered %v, want a refusal", err)
	}
}

// Once opted in, the field groups the deals it holds, and the schema names it
// with the label and choices an administrator gave it.
func TestAReportableDealFieldGroupsTheDealsThatHoldIt(t *testing.T) {
	e := integration.Setup(t)
	catalog, field := territoryField(t, e)
	optIn(t, e, catalog, field)
	column := *field.ColumnName
	ctx := e.Admin()

	answer, err := dealsByColumn(ctx, e, column)
	if err != nil {
		t.Fatalf("grouping by the reportable field: %v", err)
	}
	counts := map[string]string{}
	for _, row := range answer.Rows {
		counts[fmt.Sprint(row[column])] = fmt.Sprint(row["count"])
	}
	if want := map[string]string{"EMEA": "2", "APAC": "1"}; !maps.Equal(counts, want) {
		t.Fatalf("counts by territory %v, want %v", counts, want)
	}

	var schema analyticsquery.Schema
	if err := database.WithWorkspaceTx(ctx, e.Pool, func(tx pgx.Tx) error {
		schema, _, err = AnalyticsSchemaIn(ctx, tx)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	described := analyticsCustomFieldsOf(schema.Entities["deals-by-stage"])
	if described == nil || len(*described) != 1 {
		t.Fatalf("the schema describes %v custom fields, want the territory alone", described)
	}
	got := (*described)[0]
	if got.Name != column || got.Label != "Territory" || !slices.Equal(got.Options, []string{"EMEA", "APAC"}) ||
		got.Type != crmcontracts.AnalyticsCustomFieldTypePicklist {
		t.Errorf("the schema describes %+v, want the territory's name, label and choices", got)
	}
	document, err := analyticsVocabularyText(ctx, e.DB())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(document, column+` "Territory": EMEA, APAC`) {
		t.Errorf("the vocabulary a model reads does not name the territory and its choices:\n%s", document)
	}
	if AnalyticsSchemaFor(ctx).Version == schema.Version {
		t.Error("adding a dimension left the schema version unchanged, so an old plan reads as current")
	}
}

// Only a picklist or a yes/no field may be reportable: a free-text value is a
// group per record, and the refusal names the field.
func TestAFreeTextFieldCannotBeOptedIntoReporting(t *testing.T) {
	e := integration.Setup(t)
	catalog := customfields.NewService(e.Pool, integration.SchemaPool(t))
	admin := e.As(e.Rep1, nil, integration.CustomFieldAdminPerms)
	field, err := catalog.Create(admin, customfields.FieldSpec{
		Object: "deal", Label: "Notes", Type: customfields.TypeText, Source: "manual",
	})
	if err != nil {
		t.Fatal(err)
	}
	yes := true
	_, err = catalog.Update(admin, ids.UUID(field.Id), customfields.FieldPatch{Reportable: &yes}, nil)
	var refusal *customfields.ValidationError
	if !errors.As(err, &refusal) || len(refusal.Errors) != 1 || refusal.Errors[0].Field != "reportable" {
		t.Fatalf("opting a text field in answered %v, want a refusal naming reportable", err)
	}
}

// The resource and the vocabulary tool's reader serve one document. The two
// doors share analyticsVocabularyText today; this holds the sharing, so a
// second rendering cannot arrive under either door without failing here.
func TestTheAnalyticsResourceAndTheToolReaderServeOneDocument(t *testing.T) {
	e := integration.Setup(t)
	ctx := e.Admin()
	contents, err := analyticsSchemaResource{db: e.DB()}.ReadResource(ctx, AnalyticsSchemaURI)
	if err != nil {
		t.Fatalf("the resource: %v", err)
	}
	viaTool, err := analyticsVocabularyReader{db: e.DB()}.AnalyticsVocabularyDocument(ctx)
	if err != nil {
		t.Fatalf("the tool's reader: %v", err)
	}
	if contents.Text != viaTool {
		t.Errorf("the resource and the tool serve two documents:\nresource: %q\ntool:     %q",
			contents.Text, viaTool)
	}
	// The equality above must be over a real vocabulary, not two empty
	// strings agreeing.
	if !strings.Contains(viaTool, "group by") {
		t.Errorf("the shared document describes no population, so the equality proved nothing: %q", viaTool)
	}
}

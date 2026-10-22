// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/analyticsquery"
	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/customfields"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// A deal with no territory is a group of its own, after the named ones. The
// groups add up to every deal: none is counted twice or dropped.
func TestGroupingByACustomFieldCountsEveryDealOnce(t *testing.T) {
	e := integration.Setup(t)
	fixture := territoryDeals(t, e)
	optIn(t, e, fixture.catalog, fixture.field)
	if _, err := fixture.deals.CreateDeal(e.Admin(), deals.CreateDealInput{
		Name: "No territory yet", PipelineID: fixture.pipeline, StageID: fixture.open,
	}); err != nil {
		t.Fatal(err)
	}
	column := *fixture.field.ColumnName

	answer, err := dealsByColumn(e.Admin(), e, column)
	if err != nil {
		t.Fatal(err)
	}
	var order []string
	total := 0
	for _, row := range answer.Rows {
		order = append(order, fmt.Sprint(row[column]))
		total += countOf(t, row)
	}
	if want := []string{"APAC", "EMEA", "<nil>"}; !slices.Equal(order, want) {
		t.Errorf("groups in order %v, want the named ones by value and the empty one last %v", order, want)
	}
	all, err := dealsByColumn(e.Admin(), e, "status")
	if err != nil {
		t.Fatal(err)
	}
	everyDeal := 0
	for _, row := range all.Rows {
		everyDeal += countOf(t, row)
	}
	if total != everyDeal || total != 4 {
		t.Errorf("the territory groups count %d deals and the status groups %d, want 4 in both", total, everyDeal)
	}
}

// Once a field leaves reports, every door that asks a question gives one
// refusal. The doors are a direct run, the analytics tool and a saved question.
func TestEveryDoorRefusesAFieldThatLeftReportsTheSameWay(t *testing.T) {
	e := integration.Setup(t)
	catalog, field := territoryField(t, e)
	optIn(t, e, catalog, field)
	column := *field.ColumnName
	ctx := e.Admin()
	q := analyticsquery.Query{
		Entity: "deals-by-stage", GroupBy: []string{column},
		Measures: []analyticsquery.Measure{{Fn: analyticsquery.CountAll}},
	}
	var runID ids.UUID
	if err := database.WithWorkspaceTx(ctx, e.Pool, func(tx pgx.Tx) error {
		answer, err := RunAnalyticsQuery(ctx, tx, q, analyticsquery.Floor(1))
		if err != nil {
			return err
		}
		runID, err = SaveReportRun(ctx, tx, q, answer, analyticsquery.Floor(1))
		return err
	}); err != nil {
		t.Fatalf("saving the question while the field was in reports: %v", err)
	}

	off := false
	if _, err := catalog.Update(e.As(e.Rep1, nil, integration.CustomFieldAdminPerms),
		ids.UUID(field.Id), customfields.FieldPatch{Reportable: &off}, nil); err != nil {
		t.Fatal(err)
	}

	refusals := map[string]error{}
	refusals["run"] = database.WithWorkspaceTx(ctx, e.Pool, func(tx pgx.Tx) error {
		_, err := RunAnalyticsQuery(ctx, tx, q, analyticsquery.Floor(1))
		return err
	})
	refusals["saved"] = database.WithWorkspaceTx(ctx, e.Pool, func(tx pgx.Tx) error {
		_, err := ReadReportRun(ctx, tx, runID, analyticsquery.Floor(1))
		return err
	})
	body, err := json.Marshal(map[string]any{
		"entity": "deals-by-stage", "group_by": []string{column},
		"measures": []map[string]string{{"fn": "count"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	_, refusals["tool"] = analyticsQueryToolRunner(e.DB())(ctx, body)

	reference, ok := errors.AsType[*analyticsquery.RefusalError](refusals["run"])
	if !ok || !strings.Contains(reference.Suggest, "In reports") {
		t.Fatalf("a direct run answered %v, want a refusal naming the repair", refusals["run"])
	}
	for door, err := range refusals {
		refusal, ok := errors.AsType[*analyticsquery.RefusalError](err)
		if !ok {
			t.Errorf("the %s door answered %v, want a refusal", door, err)
			continue
		}
		if *refusal != *reference {
			t.Errorf("the %s door refused with %+v, a direct run with %+v", door, *refusal, *reference)
		}
	}
}

// A field's identity is its column, so a rename keeps a saved question
// answering, and the schema names the field by its new label.
func TestARenamedReportableFieldKeepsASavedQuestionAnswering(t *testing.T) {
	e := integration.Setup(t)
	catalog, field := territoryField(t, e)
	optIn(t, e, catalog, field)
	column := *field.ColumnName
	ctx := e.Admin()
	q := analyticsquery.Query{
		Entity: "deals-by-stage", GroupBy: []string{column},
		Measures: []analyticsquery.Measure{{Fn: analyticsquery.CountAll}},
	}
	var runID ids.UUID
	if err := database.WithWorkspaceTx(ctx, e.Pool, func(tx pgx.Tx) error {
		answer, err := RunAnalyticsQuery(ctx, tx, q, analyticsquery.Floor(1))
		if err != nil {
			return err
		}
		runID, err = SaveReportRun(ctx, tx, q, answer, analyticsquery.Floor(1))
		return err
	}); err != nil {
		t.Fatal(err)
	}
	region := "Sales region"
	if _, err := catalog.Update(e.As(e.Rep1, nil, integration.CustomFieldAdminPerms),
		ids.UUID(field.Id), customfields.FieldPatch{Label: &region}, nil); err != nil {
		t.Fatal(err)
	}

	if err := database.WithWorkspaceTx(ctx, e.Pool, func(tx pgx.Tx) error {
		run, err := ReadReportRun(ctx, tx, runID, analyticsquery.Floor(1))
		if err != nil {
			return fmt.Errorf("the saved question after the rename: %w", err)
		}
		if len(run.Answer.Rows) != 2 {
			t.Errorf("the saved question answers %v, want the two territories", run.Answer.Rows)
		}
		schema, _, err := AnalyticsSchemaIn(ctx, tx)
		if err != nil {
			return err
		}
		described := analyticsCustomFieldsOf(schema.Entities["deals-by-stage"])
		if described == nil || (*described)[0].Label != region {
			t.Errorf("the schema describes %v, want the field under its new label", described)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func countOf(t *testing.T, row map[string]any) int {
	t.Helper()
	var n int
	if _, err := fmt.Sscan(fmt.Sprint(row["count"]), &n); err != nil {
		t.Fatalf("row %v carries no count: %v", row, err)
	}
	return n
}

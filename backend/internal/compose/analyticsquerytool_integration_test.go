// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// run_analytics_query against a real database: the tool and POST
// /analytics/query are one engine, so the answer a model reads must be the
// answer the screen draws — same columns, same rows, same floor.

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/analyticsquery"
	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/agents"
	"github.com/margince/margince/backend/internal/platform/database"
)

func seedToolDeals(t *testing.T, e *integration.Env) {
	t.Helper()
	pipeline, open, _ := integration.DealFixture(t, e)
	owner := integration.OwnerConn(t)
	for i := range 6 {
		if _, err := owner.Exec(context.Background(), `
			INSERT INTO deal (pipeline_id, stage_id, name, owner_id, status, source, captured_by,
			                  amount_minor, currency)
			VALUES ($1, $2, 'Tool Deal ' || $3, $4, 'open', 'manual', 'test', 100000, 'EUR')`,
			pipeline, open, fmt.Sprintf("%d", i), e.Rep1); err != nil {
			t.Fatalf("seeding deal %d: %v", i, err)
		}
	}
}

func TestTheAnalyticsToolAndTheHTTPEngineServeOneAnswer(t *testing.T) {
	e := integration.Setup(t)
	seedToolDeals(t, e)
	ctx := e.Admin()
	run := analyticsQueryToolRunner(e.DB())

	raw, err := run(ctx, json.RawMessage(
		`{"entity":"deals-by-stage","measures":[{"fn":"count"}],"save":true}`))
	if err != nil {
		t.Fatalf("the tool refused a well-formed count: %v", err)
	}
	var got agents.AnalyticsQueryResult
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatal(err)
	}
	if len(got.Rows) == 0 {
		t.Fatal("the tool answered no rows over six seeded deals")
	}
	if got.RunID == nil {
		t.Fatal("save was set and the answer carries no run_id — nothing for a report to cite")
	}
	if got.SchemaVersion == "" {
		t.Error("the answer names no schema version, so two answers cannot be compared")
	}

	// The HTTP engine's answer to the same question, through the same seam.
	var twin AnalyticsAnswer
	if err := database.WithWorkspaceTx(ctx, e.Pool, func(tx pgx.Tx) error {
		var err error
		twin, err = RunAnalyticsQuery(ctx, tx, analyticsquery.Query{
			Entity:   "deals-by-stage",
			Measures: []analyticsquery.Measure{{Fn: analyticsquery.CountAll}},
		}, analyticsquery.DefaultFloor)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if len(twin.Rows) != len(got.Rows) {
		t.Errorf("the tool served %d rows and the HTTP engine %d — one engine, two answers",
			len(got.Rows), len(twin.Rows))
	}
	if twin.SchemaVersion != got.SchemaVersion {
		t.Errorf("schema versions differ: tool %q, engine %q", got.SchemaVersion, twin.SchemaVersion)
	}
}

// An argument key the engine does not serve is refused by name, not dropped.
func TestTheAnalyticsToolRefusesAnUnservedArgument(t *testing.T) {
	e := integration.Setup(t)
	ctx := e.Admin()
	run := analyticsQueryToolRunner(e.DB())
	if _, err := run(ctx, json.RawMessage(
		`{"entity":"deals-by-stage","measures":[{"fn":"count"}],"as_of":"2024-01-01"}`)); err == nil {
		t.Fatal("an argument this engine does not serve was silently dropped — the caller " +
			"asked for a historical answer and would have been served the current one")
	}
}

// Exactly one JSON value: trailing content is a caller who thinks they sent
// something this never read.
func TestTheAnalyticsToolRefusesTrailingContent(t *testing.T) {
	e := integration.Setup(t)
	run := analyticsQueryToolRunner(e.DB())
	if _, err := run(e.Admin(), json.RawMessage(
		`{"entity":"deals-by-stage","measures":[{"fn":"count"}]} {"save":true}`)); err == nil {
		t.Fatal("trailing content after the arguments was silently ignored")
	}
}

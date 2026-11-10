// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// The money a phase of projects is worth, folded the way its rows read.
//
// A project with no deals shows zero on its own row, so an average or a minimum
// over the phase counts it as zero. A project with a deal no rate can price
// leaves the phase's sum, average, minimum and maximum without an answer.
func TestAPhasesMoneyFoldsItsProjectsAsTheirRowsRead(t *testing.T) {
	e := setupForecast(t)
	company := e.seedID(t, `INSERT INTO company (id, display_name, source, captured_by)
		VALUES ($1, 'Fold Co', 'manual', 'human:x')`)
	// A deal may name only a company on its project, so each project carries
	// the company as its customer.
	project := func(name, phase string) ids.UUID {
		id := e.seedID(t, `INSERT INTO project (id, name, company_id, phase, source, captured_by)
			VALUES ($1, $2, $3, $4, 'manual', 'human:x')`, name, company, phase)
		e.seedID(t, `INSERT INTO relationship (id, kind, project_id, company_id, role, source, captured_by)
			VALUES ($1, 'project_company', $2, $3, 'customer', 'manual', 'human:x')`, id, company)
		return id
	}
	openDeal := func(on ids.UUID, amountMinor int64, currency string) {
		e.seedID(t, `INSERT INTO deal
			(id, name, pipeline_id, stage_id, amount_minor, currency, status,
			 expected_close_date, project_id, company_id, source, captured_by)
			VALUES ($1, 'Fold deal', $2, $3, $4, $5, 'open',
			        (now() + interval '30 days')::date, $6, $7, 'manual', 'human:x')`,
			e.pipeline, e.stages[pipelineTestStage], amountMinor, currency, on, company)
	}
	openDeal(project("Priced delivery", "delivering"), 1_000, "EUR")
	project("Dealless delivery", "delivering")
	openDeal(project("Priced pursuit", "pursuing"), 400, "EUR")
	openDeal(project("Unrated pursuit", "pursuing"), 200, "CHF")

	reader := principal.WithActor(principal.WithWorkspaceID(context.Background(), e.WS), principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:project-reader", UserID: e.Rep1,
		Permissions: principal.Permissions{
			Objects: map[string]principal.ObjectGrant{
				"project": {Read: true}, "deal": {Read: true}, "installation_settings": {Read: true},
			},
			RowScope: principal.RowScopeAll,
		},
	})
	result := e.runReport(reader, t, "projects-by-phase", `{"group_by":["phase"],"aggregates":[
		{"fn":"sum","field":"open_deal_value_minor","as":"total"},
		{"fn":"avg","field":"open_deal_value_minor","as":"mean"},
		{"fn":"min","field":"open_deal_value_minor","as":"least"},
		{"fn":"max","field":"open_deal_value_minor","as":"most"}]}`)
	phases := map[string]map[string]any{}
	for _, row := range result.Rows {
		phases[row["phase"].(string)] = row
	}

	for key, want := range map[string]float64{"total": 1_000, "mean": 500, "least": 0, "most": 1_000} {
		if got, ok := moneyCell(t, phases["delivering"], key); !ok || got != want {
			t.Errorf("delivering %s = %v (answered %v), want %v: the dealless project counts as zero", key, got, ok, want)
		}
	}
	for _, key := range []string{"total", "mean", "least", "most"} {
		if got, ok := moneyCell(t, phases["pursuing"], key); ok {
			t.Errorf("pursuing %s answered %v, want no answer: one project's deal has no rate", key, got)
		}
	}
}

// moneyCell reads one aggregate cell, and false for a null one.
func moneyCell(t *testing.T, row map[string]any, key string) (float64, bool) {
	t.Helper()
	if row == nil {
		t.Fatalf("the phase is missing from the report, so %q proves nothing", key)
	}
	if row[key] == nil {
		return 0, false
	}
	num, ok := row[key].(json.Number)
	if !ok {
		t.Fatalf("cell %q = %v (%T), want a number", key, row[key], row[key])
	}
	v, err := num.Float64()
	if err != nil {
		t.Fatalf("cell %q = %v: %v", key, num, err)
	}
	return v, true
}

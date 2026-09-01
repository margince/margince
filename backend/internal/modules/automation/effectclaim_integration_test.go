// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package automation

// The multi-instance fan-out invariant, against real Postgres: the engine
// fires route_lead once per enabled instance, and before the effect-level
// claim (automation_effect_claim) two instances with identical params both
// minted the same follow-up task off one lead.created. One identical
// effect applies once; different params still each apply; a redelivery of
// the whole event applies nothing new.

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/platform/database"
	kevents "github.com/margince/margince/backend/internal/shared/kernel/events"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
)

// countingProvider records every Create the engine's executor performs.
// The provider is a true boundary here — what is under test is the claim
// and the fan-out, not the datasource write — and only Create is ever
// reached, so the embedded nil interface backs the unused methods.
type countingProvider struct {
	datasource.SystemOfRecordProvider
	created []datasource.CreateInput
}

func (p *countingProvider) Create(_ context.Context, in datasource.CreateInput) (datasource.EntityRef, error) {
	p.created = append(p.created, in)
	return datasource.EntityRef{Type: in.EntityType, ID: ids.NewV7()}, nil
}

func (p *countingProvider) Read(_ context.Context, ref datasource.EntityRef) (datasource.Record, error) {
	return ownerlessRecord(ref), nil
}

// leadCreatedEnvelope is one lead.created delivery for a fixed lead.
func leadCreatedEnvelope(lead ids.UUID) kevents.Envelope {
	return kevents.Envelope{
		EventID:    ids.NewV7(),
		Type:       "lead.created",
		OccurredAt: time.Date(2026, 8, 28, 9, 0, 0, 0, time.UTC),
		Entity:     kevents.EntityRef{Type: "lead", ID: lead},
	}
}

// engineOverClaims builds the engine with the real claim store and the
// counting provider, registering the shipped starters exactly as compose
// does.
func engineOverClaims(fx *autoFixture, provider *countingProvider) *WorkflowEngine {
	db := database.BindTo(fx.pool, ids.From[ids.WorkspaceKind](fx.ws))
	engine := NewWorkflowEngine(db, fixtureResolver{})
	ex := Executors{Provider: provider, Claims: NewEffectClaims(db)}
	for _, handler := range StarterWorkflows(ex) {
		engine.RegisterWorkflow(handler)
	}
	return engine
}

func TestTwoIdenticalInstancesMintOneTask(t *testing.T) {
	fx := setupAutomationDB(t)
	fx.seedAutomation(t, routeLeadName)
	fx.seedAutomation(t, routeLeadName)

	provider := &countingProvider{}
	engine := engineOverClaims(fx, provider)
	if err := engine.HandleEvent(context.Background(), leadCreatedEnvelope(ids.NewV7())); err != nil {
		t.Fatalf("HandleEvent: %v", err)
	}

	if len(provider.created) != 1 {
		t.Fatalf("two identical route_lead instances created %d records, want exactly 1", len(provider.created))
	}
	// Both firings still recorded a run — history per instance stays intact —
	// and exactly one of them says its create folded into the sibling's.
	rows, err := fx.owner.Query(context.Background(),
		`SELECT status, coalesce(applied::text, '') FROM workflow_run WHERE handler = $1`, routeLeadName)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	runs, deduplicated := 0, 0
	for rows.Next() {
		var status, applied string
		if err := rows.Scan(&status, &applied); err != nil {
			t.Fatal(err)
		}
		if status != "applied" {
			t.Errorf("run status = %q, want applied", status)
		}
		runs++
		// jsonb::text prints a space after the colon; match both spellings
		// so the assertion is about the flag, not Postgres's printer.
		if strings.Contains(applied, `"deduplicated": true`) || strings.Contains(applied, `"deduplicated":true`) {
			deduplicated++
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if runs != 2 {
		t.Fatalf("recorded %d runs, want 2 (one per instance)", runs)
	}
	if deduplicated != 1 {
		t.Fatalf("%d runs annotated deduplicated, want exactly 1", deduplicated)
	}
}

func TestDifferentParamsEachApply(t *testing.T) {
	fx := setupAutomationDB(t)
	// Two genuinely different parameterizations: the due dates differ, so
	// each is its own effect and both must land.
	for _, params := range []string{`{"due_in_days": 1}`, `{"due_in_days": 3}`} {
		id := ids.New[ids.AutomationKind]()
		fx.exec(t, `
			INSERT INTO automation (id, key, name, trigger, action, params, enabled, tier)
			VALUES ($1, $2, $2, '{"event_type":"test"}', '{"kind":"test"}', $3::jsonb, true, 'auto_execute')`,
			id, routeLeadName, params)
	}

	provider := &countingProvider{}
	engine := engineOverClaims(fx, provider)
	if err := engine.HandleEvent(context.Background(), leadCreatedEnvelope(ids.NewV7())); err != nil {
		t.Fatalf("HandleEvent: %v", err)
	}
	if len(provider.created) != 2 {
		t.Fatalf("two differently-parameterized instances created %d records, want 2", len(provider.created))
	}
}

func TestRedeliveryAppliesNothingNew(t *testing.T) {
	fx := setupAutomationDB(t)
	fx.seedAutomation(t, routeLeadName)

	provider := &countingProvider{}
	engine := engineOverClaims(fx, provider)
	env := leadCreatedEnvelope(ids.NewV7())
	for range 2 {
		if err := engine.HandleEvent(context.Background(), env); err != nil {
			t.Fatalf("HandleEvent: %v", err)
		}
	}
	if len(provider.created) != 1 {
		t.Fatalf("a redelivered event created %d records, want 1", len(provider.created))
	}
}

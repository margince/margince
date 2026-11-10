// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package automation

import (
	"context"
	"errors"
	"testing"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	kevents "github.com/margince/margince/backend/internal/shared/kernel/events"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/workflow"
)

const (
	leadUpdated = "lead.updated"
	writer      = "agent:writer"
)

func actingAs(actor string) context.Context {
	ctx := principal.WithCorrelationID(context.Background(), ids.NewV7())
	return principal.WithActor(ctx, principal.Principal{Type: principal.PrincipalAgent, ID: actor})
}

// wroteLead emits, through the real writer, the event the writer's write to a
// lead leaves. It returns the envelope as the bus delivers it.
func (fx *autoFixture) wroteLead(t *testing.T, lead ids.UUID) kevents.Envelope {
	t.Helper()
	ctx := actingAs(writer)
	tx, err := fx.owner.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	if err := storekit.Emit(ctx, tx, ids.NewV7(), leadUpdated, "lead", lead, map[string]string{"field": "status"}); err != nil {
		t.Fatalf("Emit: %v", err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit: %v", err)
	}
	var env kevents.Envelope
	if err := fx.owner.QueryRow(ctx, `SELECT envelope FROM event_outbox
		WHERE envelope->'entity'->>'id' = $1`, lead.String()).Scan(&env); err != nil {
		t.Fatalf("reading the emitted event: %v", err)
	}
	return env
}

func (fx *autoFixture) delivered(t *testing.T, env kevents.Envelope) {
	t.Helper()
	fx.exec(t, `UPDATE event_outbox SET published_at = now() WHERE envelope->>'event_id' = $1`, env.EventID.String())
}

func running(t *testing.T, engine *WorkflowEngine, actor string, lead ids.UUID) bool {
	t.Helper()
	got, err := engine.PendingFor(actingAs(actor), []ids.Ref{{Type: "lead", ID: lead}})
	if err != nil {
		t.Fatalf("PendingFor: %v", err)
	}
	return got
}

func TestAWriteIsPendingUntilItsAutomationRecordsAnOutcome(t *testing.T) {
	fx := setupAutomationDB(t)
	const handler, actor = "reacts_to_a_write", writer
	fx.seedAutomation(t, handler)
	engine := engineOverScripted(fx, scriptedWorkflow{name: handler, trigger: leadUpdated})
	lead := ids.NewV7()
	env := fx.wroteLead(t, lead)

	if !running(t, engine, actor, lead) {
		t.Fatal("an undelivered write read as settled")
	}
	fx.delivered(t, env)
	if !running(t, engine, actor, lead) {
		t.Fatal("a delivered write its automation has not handled read as settled")
	}
	if running(t, engine, "agent:someone_else", lead) {
		t.Fatal("another caller's listing read a write it did not make as pending")
	}
	if running(t, engine, actor, ids.NewV7()) {
		t.Fatal("a record nobody wrote read as pending")
	}
	// The scripted Apply records no actions, the outcome a run with nothing to do leaves.
	if err := engine.HandleEvent(context.Background(), env); err != nil {
		t.Fatalf("HandleEvent: %v", err)
	}
	if running(t, engine, actor, lead) {
		t.Fatal("a write whose automation recorded its outcome read as pending")
	}
}

func TestAFailedRunIsNotPending(t *testing.T) {
	fx := setupAutomationDB(t)
	const handler, actor = "fails", writer
	fx.seedAutomation(t, handler)
	engine := engineOverScripted(fx, scriptedWorkflow{
		name: handler, trigger: leadUpdated,
		apply: func(workflow.Event) (workflow.RunResult, error) {
			return workflow.RunResult{}, errors.New("unreachable")
		},
	})
	lead := ids.NewV7()
	env := fx.wroteLead(t, lead)
	fx.delivered(t, env)
	if err := engine.HandleEvent(context.Background(), env); err == nil {
		t.Fatal("the scripted Apply failed, so HandleEvent must surface it")
	}
	if running(t, engine, actor, lead) {
		t.Fatal("a run that failed read as pending")
	}
}

func TestAClaimWithNoOutcomeIsPendingOnlyWithinTheReclaimWindow(t *testing.T) {
	fx := setupAutomationDB(t)
	const handler, actor = "crashes", writer
	automation := fx.seedAutomation(t, handler)
	h := scriptedWorkflow{name: handler, trigger: leadUpdated}
	engine := engineOverScripted(fx, h)
	lead := ids.NewV7()
	env := fx.wroteLead(t, lead)
	fx.delivered(t, env)
	// A worker that died between claimRun and recordApplyOutcome leaves this row.
	ev := workflowEvent(env, fx.ws)
	ev.AutomationID = automation.UUID
	fx.exec(t, `INSERT INTO workflow_run (handler, idempotency_key, trigger_event, planned, status)
		VALUES ($1, $2, $3, '[]', 'applied')`, handler, runKey(h, ev), env.EventID)
	if !running(t, engine, actor, lead) {
		t.Fatal("a claim still within the reclaim window read as settled")
	}
	fx.exec(t, `UPDATE workflow_run SET created_at = now() - interval '1 hour' WHERE handler = $1`, handler)
	if running(t, engine, actor, lead) {
		t.Fatal("a claim older than the reclaim window still read as pending")
	}
}

func TestAWriteNoEnabledAutomationMatchesIsNotPending(t *testing.T) {
	fx := setupAutomationDB(t)
	const actor = writer
	fx.seedAutomation(t, "declines")
	engine := engineOverScripted(fx, scriptedWorkflow{
		name: "declines", trigger: leadUpdated,
		match: func(workflow.Event) (bool, error) { return false, nil },
	})
	engine.RegisterWorkflow(scriptedWorkflow{name: "never_enabled", trigger: leadUpdated})
	lead := ids.NewV7()
	fx.delivered(t, fx.wroteLead(t, lead))

	if running(t, engine, actor, lead) {
		t.Fatal("a write read as pending though one handler declines it and the other has no enabled instance")
	}
}

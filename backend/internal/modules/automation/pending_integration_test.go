// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package automation

import (
	"context"
	"encoding/json"
	"testing"

	kevents "github.com/margince/margince/backend/internal/shared/kernel/events"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/workflow"
)

// writtenBy puts one envelope in event_outbox as the given actor's write on a
// lead, the shape storekit.Emit leaves, and returns it.
func (fx *autoFixture) writtenBy(t *testing.T, actor string, lead ids.UUID) kevents.Envelope {
	t.Helper()
	env := kevents.Envelope{
		EventID: ids.NewV7(), Type: scriptedTrigger, Version: 1, OccurredAt: retryInstant,
		Actor:  kevents.Actor{Type: "agent", ID: actor},
		Entity: kevents.EntityRef{Type: "lead", ID: lead},
	}
	raw, err := json.Marshal(env)
	if err != nil {
		t.Fatalf("marshalling the envelope: %v", err)
	}
	fx.exec(t, `INSERT INTO event_outbox (id, stream, envelope, published_at) VALUES ($1, 'crm.lead', $2::jsonb, now())`,
		env.EventID, raw)
	return env
}

func actingAs(actor string) context.Context {
	return principal.WithActor(context.Background(), principal.Principal{Type: principal.PrincipalAgent, ID: actor})
}

func pendingOn(t *testing.T, engine *WorkflowEngine, actor string, lead ids.UUID) int {
	t.Helper()
	n, err := engine.PendingFor(actingAs(actor), []ids.Ref{{Type: "lead", ID: lead}})
	if err != nil {
		t.Fatalf("PendingFor: %v", err)
	}
	return n
}

func TestAWriteReadsAsPendingUntilItsAutomationRecordsAnOutcome(t *testing.T) {
	fx := setupAutomationDB(t)
	const handler, actor = "reacts_to_a_write", "agent:writer"
	fx.seedAutomation(t, handler)
	engine := engineOverScripted(fx, scriptedWorkflow{name: handler})
	lead := ids.NewV7()
	env := fx.writtenBy(t, actor, lead)

	if n := pendingOn(t, engine, actor, lead); n != 1 {
		t.Fatalf("before the automation ran, %d runs read as pending, want 1", n)
	}
	if n := pendingOn(t, engine, "agent:someone_else", lead); n != 0 {
		t.Fatalf("another caller's listing counted %d pending runs on a write it did not make, want 0", n)
	}
	if n, err := engine.PendingFor(actingAs(actor), []ids.Ref{{Type: "lead", ID: ids.NewV7()}}); err != nil || n != 0 {
		t.Fatalf("a record nobody wrote read %d pending (%v), want 0", n, err)
	}
	if err := engine.HandleEvent(context.Background(), env); err != nil {
		t.Fatalf("HandleEvent: %v", err)
	}
	if n := pendingOn(t, engine, actor, lead); n != 0 {
		t.Fatalf("after the automation recorded its outcome, %d runs read as pending, want 0", n)
	}
}

func TestAWriteNoEnabledAutomationMatchesIsNotPending(t *testing.T) {
	fx := setupAutomationDB(t)
	const actor = "agent:writer"
	fx.seedAutomation(t, "declines")
	engine := engineOverScripted(fx, scriptedWorkflow{
		name: "declines", match: func(workflow.Event) (bool, error) { return false, nil },
	})
	engine.RegisterWorkflow(scriptedWorkflow{name: "never_enabled"})
	lead := ids.NewV7()
	fx.writtenBy(t, actor, lead)

	if n := pendingOn(t, engine, actor, lead); n != 0 {
		t.Fatalf("%d runs read as pending, want 0: one handler declines the event and the other has no enabled instance", n)
	}
}

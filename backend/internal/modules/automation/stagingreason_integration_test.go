// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package automation

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/margince/margince/backend/internal/platform/database"
	kevents "github.com/margince/margince/backend/internal/shared/kernel/events"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
	"github.com/margince/margince/backend/internal/shared/ports/workflow"
)

type reasonWorkflow struct {
	scriptedWorkflow
	approvals Approvals
}

func (h reasonWorkflow) Plan(_ context.Context, ev workflow.Event) (workflow.Effect, error) {
	return workflow.Effect{Actions: []workflow.Action{{Kind: workflow.ActionEmitFlowEvent,
		Target: ev.Entity, Args: json.RawMessage(`{"question":"Has the account review been completed?"}`)}}}, nil
}

func (h reasonWorkflow) Apply(ctx context.Context, _ workflow.Event, effect workflow.Effect, _ *workflow.ApprovalToken) (workflow.RunResult, error) {
	_, err := ApplyActions(ctx, Executors{Approvals: h.approvals}, effect)
	return workflow.RunResult{}, err
}

func TestTheEnginePassesTheStoredRuleNameToItsStagedApproval(t *testing.T) {
	fx := setupAutomationDB(t)
	fake := &fakeApprovals{id: ids.New[ids.ApprovalKind]()}
	h := reasonWorkflow{scriptedWorkflow: scriptedWorkflow{name: "review"}, approvals: fake}
	id := fx.seedAutomation(t, h.name)
	fx.exec(t, `UPDATE automation SET name = @name WHERE id = @id`, pgx.StrictNamedArgs{"name": "Quarterly review", "id": id})
	engine := NewWorkflowEngine(database.BindTo(fx.pool, ids.From[ids.WorkspaceKind](fx.ws)), nil)
	engine.RegisterWorkflow(h)
	at := time.Date(2026, 7, 1, 9, 0, 0, 0, time.UTC)
	if err := engine.HandleEvent(context.Background(), kevents.Envelope{EventID: ids.NewV7(), Type: scriptedTrigger, OccurredAt: at,
		Entity: kevents.EntityRef{Type: "deal", ID: ids.NewV7()}}); err != nil {
		t.Fatal(err)
	}
	if len(fake.calls) != 1 || fake.calls[0].Why == nil {
		t.Fatalf("the engine staged no reason: %+v", fake.calls)
	}
	text, err := fake.calls[0].Why.Render(textlang.English)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Quarterly review", scriptedTrigger, at.Format(time.RFC3339)} {
		if !strings.Contains(text, want) {
			t.Errorf("reason %q omits %q", text, want)
		}
	}
}

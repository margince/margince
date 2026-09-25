// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package ai

// A decision attempt against the real ai_call table: written through the
// router's own recorder, read back through the trace screen's store, left out
// of the totals that price an LLM re-run, and priced on a lane the sheet
// accepts.

import (
	"context"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// diagnosticsReader is a human allowed to read the call trace in one workspace.
func diagnosticsReader(ws ids.UUID) context.Context {
	return principal.WithActor(principal.WithWorkspaceID(context.Background(), ws), principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:decision-trace-test",
		Permissions: principal.Permissions{
			RoleKeys: []string{"fixture"},
			Objects:  map[string]principal.ObjectGrant{"ai_diagnostics": {Read: true}},
		},
	})
}

// recordDecisionThenLadder writes one logical call the way Decide does when a
// decision falls back: a decision attempt, then the completion that answered.
func recordDecisionThenLadder(ctx context.Context, t *testing.T, meter *CallMeter) {
	t.Helper()
	logical := ids.NewV7()
	if err := meter.Record(ctx, []Call{
		{
			LogicalCallID: logical, Attempt: 1, Kind: callKindDecision, Task: TaskSiteTriage, Tier: TierDecideLane,
			Provider: providerOpenRouterDecision, ModelID: "typesafe/jev-1.13", RequestFingerprint: "",
			TokensIn: 425, ServedModel: "typesafe/jev-1.13-20260917", ServedIdentitySource: servedIdentitySourceResponse,
			ServedProvider: "TypeSafe",
		},
		{
			LogicalCallID: logical, Attempt: 2, IsTerminal: true, Kind: callKindCompletion, Task: TaskSiteTriage,
			Tier: TierCheapCloud, Provider: providerGemini, ModelID: "gemini-3.1-flash-lite", RequestFingerprint: "fp-triage",
			AttemptReason: attemptReasonDecisionBelowFloor, TokensIn: 900, TokensOut: 40,
			ServedModel: "gemini-3.1-flash-lite", ServedIdentitySource: servedIdentitySourceResponse,
		},
	}); err != nil {
		t.Fatalf("recording the decision call: %v", err)
	}
}

func TestADecisionCallRoundTripsTheTrace(t *testing.T) {
	env := setupRateStore(t)
	ws, ctx := env.seedWorkspace(context.Background(), t)
	recordDecisionThenLadder(ctx, t, NewCallMeter(env.dbFor(ws)))

	reader := NewCallReadStore(env.dbFor(ws))
	readCtx := diagnosticsReader(ws)
	page, err := reader.ListCalls(readCtx, nil, nil, nil)
	if err != nil || len(page.Items) != 1 {
		t.Fatalf("ListCalls: %d items, %v", len(page.Items), err)
	}
	if page.Items[0].Kind != callKindCompletion {
		t.Errorf("the terminal row's kind = %q, want completion", page.Items[0].Kind)
	}
	detail, err := reader.GetCall(readCtx, page.Items[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(detail.Attempts) != 2 {
		t.Fatalf("attempts = %+v, want the decision and the completion", detail.Attempts)
	}
	first, second := detail.Attempts[0], detail.Attempts[1]
	if first.Kind != callKindDecision || first.Tier != string(TierDecideLane) ||
		first.Provider != providerOpenRouterDecision || first.ModelID != "typesafe/jev-1.13" || first.TokensIn != 425 {
		t.Errorf("decision attempt = %+v", first)
	}
	if second.Kind != callKindCompletion || second.Tier != string(TierCheapCloud) || second.AttemptReason != attemptReasonDecisionBelowFloor {
		t.Errorf("completion attempt = %+v", second)
	}
}

// The totals price what the task's LLM ladder would cost to re-run, so a
// decision row in them would bill Jev's tokens at a chat model's rate.
func TestServedTaskTotalsLeavesDecisionCallsOut(t *testing.T) {
	env := setupRateStore(t)
	ws, ctx := env.seedWorkspace(context.Background(), t)
	recordDecisionThenLadder(ctx, t, NewCallMeter(env.dbFor(ws)))

	totals, err := NewCallReadStore(env.dbFor(ws)).ServedTaskTotals(ctx, []Task{TaskSiteTriage}, time.Now().Add(-time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if len(totals) != 1 || totals[0].Tier != TierCheapCloud || totals[0].TokensIn != 900 {
		t.Fatalf("totals = %+v, want the completion slice alone", totals)
	}
}

func TestTheRateLaneAcceptsDecisions(t *testing.T) {
	env := setupRateStore(t)
	ws, _ := env.seedWorkspace(context.Background(), t)
	ctx := laneWriterCtx(ws)
	store := env.storeFor(ws)

	row, err := store.SetModelRate(ctx, SetModelRateInput{
		Provider: providerOpenRouterDecision, ModelID: "typesafe/jev-1.13",
		InputUsd: "0.042", OutputUsd: "0", CacheReadUsd: "0", CacheWriteUsd: "0", Lane: LaneDecisions,
	})
	if err != nil {
		t.Fatalf("SetModelRate: %v", err)
	}
	if row.Lane != LaneDecisions || laneOf(ctx, t, store, providerOpenRouterDecision, "typesafe/jev-1.13") != LaneDecisions {
		t.Errorf("filed as %q, want %q", row.Lane, LaneDecisions)
	}
}

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
	"errors"
	"reflect"
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
			Provider: providerJevCompatible, ModelID: "typesafe/jev-1.13", RequestFingerprint: "",
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
		first.Provider != providerJevCompatible || first.ModelID != "typesafe/jev-1.13" || first.TokensIn != 425 {
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
		Provider: providerJevCompatible, ModelID: "typesafe/jev-1.13",
		InputUsd: "0.042", OutputUsd: "0", CacheReadUsd: "0", CacheWriteUsd: "0", Lane: LaneDecisions,
	})
	if err != nil {
		t.Fatalf("SetModelRate: %v", err)
	}
	if row.Lane != LaneDecisions || laneOf(ctx, t, store, providerJevCompatible, "typesafe/jev-1.13") != LaneDecisions {
		t.Errorf("filed as %q, want %q", row.Lane, LaneDecisions)
	}
}

// The list row is all a reader sees before opening a call, so a fallback whose
// terminal attempt is a completion must still say a decision model was asked.
func TestTheTraceListSaysWhichCallsAskedADecisionModel(t *testing.T) {
	env := setupRateStore(t)
	ws, ctx := env.seedWorkspace(context.Background(), t)
	meter := NewCallMeter(env.dbFor(ws))
	recordDecisionThenLadder(ctx, t, meter)
	if err := meter.Record(ctx, []Call{{
		LogicalCallID: ids.NewV7(), Attempt: 1, IsTerminal: true, Kind: callKindCompletion, Task: TaskEnrich,
		Tier: TierCheapCloud, Provider: providerGemini, ModelID: "gemini-3.1-flash-lite", RequestFingerprint: "fp-enrich",
		TokensIn: 300, TokensOut: 20, ServedModel: "gemini-3.1-flash-lite", ServedIdentitySource: servedIdentitySourceResponse,
	}}); err != nil {
		t.Fatalf("recording the ordinary call: %v", err)
	}

	reader := NewCallReadStore(env.dbFor(ws))
	page, err := reader.ListCalls(diagnosticsReader(ws), nil, nil, nil)
	if err != nil || len(page.Items) != 2 {
		t.Fatalf("ListCalls: %d items, %v", len(page.Items), err)
	}
	attempted := map[string]bool{}
	for _, item := range page.Items {
		attempted[item.Task] = item.DecisionAttempted
	}
	if !attempted[string(TaskSiteTriage)] {
		t.Errorf("the fallback call reads decision_attempted = false, want true")
	}
	if attempted[string(TaskEnrich)] {
		t.Errorf("the ordinary call reads decision_attempted = true, want false")
	}
}

// decidedDetail runs one Decide through the router with the real ai_call
// writer behind it, payload capture on, and reads the call back as the trace
// detail serves it.
func decidedDetail(t *testing.T, task Task, reply decisionReply) CallDetail {
	t.Helper()
	env := setupRateStore(t)
	ws, ctx := env.seedWorkspace(context.Background(), t)
	f := newDecideFixture(t, &scriptedDecider{replies: []decisionReply{reply}}, 0)
	f.router.calls = NewCallMeter(env.dbFor(ws))
	f.router.capturePayloads = true
	if _, _, err := f.router.Decide(ctx, task, "triage", triageQuestion, triageLLMRequest, acceptAnything, floorGate); err != nil {
		t.Fatal(err)
	}
	reader := NewCallReadStore(env.dbFor(ws))
	readCtx := diagnosticsReader(ws)
	page, err := reader.ListCalls(readCtx, nil, nil, nil)
	if err != nil || len(page.Items) != 1 {
		t.Fatalf("ListCalls: %d items, %v", len(page.Items), err)
	}
	detail, err := reader.GetCall(readCtx, page.Items[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	return detail
}

// assertAnswers holds each attempt's decision answer, in order, to want.
func assertAnswers(t *testing.T, attempts []CallAttempt, want ...*DecisionAnswer) {
	t.Helper()
	if len(attempts) != len(want) {
		t.Fatalf("attempts = %+v, want %d", attempts, len(want))
	}
	for i, attempt := range attempts {
		if !reflect.DeepEqual(attempt.DecisionAnswer, want[i]) {
			t.Errorf("attempt %d (%s) answer = %+v, want %+v", attempt.Attempt, attempt.Kind, attempt.DecisionAnswer, want[i])
		}
	}
}

// The answer a floor is tuned from is the one that did not stand, and it is
// kept on the decision row although that row is not the terminal one.
func TestAFallenBackDecisionKeepsItsAnswer(t *testing.T) {
	detail := decidedDetail(t, TaskSiteTriage, answered("company", 0.62))

	assertAnswers(t, detail.Attempts, &DecisionAnswer{Choice: "company", Confidence: 0.62}, nil)
	decided := detail.Attempts[0]
	if decided.IsTerminal || decided.ServedModel != "typesafe/jev-1.13-20260917" || decided.ServedProvider != "TypeSafe" {
		t.Errorf("decision attempt = %+v, want non-terminal and served by TypeSafe", decided)
	}
	if detail.LogicalCallID.IsZero() {
		t.Error("the call detail carries no logical call id")
	}
}

func TestAnAcceptedDecisionKeepsItsAnswer(t *testing.T) {
	detail := decidedDetail(t, TaskSiteTriage, answered("parked", 0.95))

	assertAnswers(t, detail.Attempts, &DecisionAnswer{Choice: "parked", Confidence: 0.95})
	if detail.Payload == nil {
		t.Error("the accepted decision's payload was not captured")
	}
}

// A failed call answered nothing, and a zero confidence would claim it had.
func TestAnErroredDecisionRecordsNoAnswer(t *testing.T) {
	detail := decidedDetail(t, TaskSiteTriage, decisionReply{err: errors.New("down")})

	assertAnswers(t, detail.Attempts, nil, nil)
	if sentinel := detail.Attempts[0].ErrorSentinel; sentinel == nil || *sentinel != "provider_error" {
		t.Errorf("decision attempt sentinel = %v, want provider_error", sentinel)
	}
}

// The answer is a label and a number, never the prompt, so a task that may
// keep no payload keeps it as well.
func TestANoPayloadTaskStillKeepsTheDecisionAnswer(t *testing.T) {
	if !NoPayload(TaskAccountScan) {
		t.Fatalf("%s is no longer no_payload; pick a task that is", TaskAccountScan)
	}
	detail := decidedDetail(t, TaskAccountScan, answered("company", 0.62))

	assertAnswers(t, detail.Attempts, &DecisionAnswer{Choice: "company", Confidence: 0.62}, nil)
	if detail.Payload != nil {
		t.Error("a no_payload task's call captured a payload")
	}
}

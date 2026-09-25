// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package ai

// The decision summary against the real ai_call table, every logical call
// written through the router's own recorder in the shapes Decide leaves.

import (
	"context"
	"errors"
	"maps"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func triageDecisionRow(logical ids.UUID, terminal bool, sentinel string) Call {
	return Call{
		LogicalCallID: logical, Attempt: 1, IsTerminal: terminal, Kind: callKindDecision, Task: TaskSiteTriage,
		Tier: TierDecideLane, Provider: providerJevCompatible, ModelID: "typesafe/jev-1.13",
		TokensIn: 400, ErrorSentinel: sentinel,
	}
}

func triageLadderRow(logical ids.UUID, attempt int, reason string) Call {
	return Call{
		LogicalCallID: logical, Attempt: attempt, IsTerminal: true, Kind: callKindCompletion, Task: TaskSiteTriage,
		Tier: TierCheapCloud, Provider: providerGemini, ModelID: "gemini-3.1-flash-lite", RequestFingerprint: "fp-triage",
		AttemptReason: reason, TokensIn: 900, TokensOut: 40,
	}
}

// triageFailedOverWalk is a fallback walk whose first rung failed: the walk's
// reason sits on that rung, and the rung that answered reads provider_error.
func triageFailedOverWalk(logical ids.UUID, reason string) []Call {
	first := triageLadderRow(logical, 2, reason)
	first.IsTerminal, first.ErrorSentinel, first.TokensOut = false, "provider_unavailable", 0
	answered := triageLadderRow(logical, 3, attemptReasonProviderError)
	answered.Tier = TierPremium
	return []Call{first, answered}
}

// recordDecisionOutcomes writes one logical call of each shape Decide leaves,
// plus an ordinary completion that never consulted the decision lane.
func recordDecisionOutcomes(ctx context.Context, t *testing.T, meter *CallMeter) {
	t.Helper()
	decided, belowFloor, errored, refused, plain := ids.NewV7(), ids.NewV7(), ids.NewV7(), ids.NewV7(), ids.NewV7()
	logicalCalls := [][]Call{
		{triageDecisionRow(decided, true, "")},
		append([]Call{triageDecisionRow(belowFloor, false, "")}, triageFailedOverWalk(belowFloor, attemptReasonDecisionBelowFloor)...),
		// The errored decision's own row and the reason its ladder carries are
		// one consultation, not two.
		{triageDecisionRow(errored, false, "provider_error"), triageLadderRow(errored, 2, attemptReasonDecisionError)},
		// Refused before any call: the ladder's row is all there is.
		{triageLadderRow(refused, 1, attemptReasonDecisionUncertified)},
		{triageLadderRow(plain, 1, "")},
	}
	for _, calls := range logicalCalls {
		if err := meter.Record(ctx, calls); err != nil {
			t.Fatalf("recording %+v: %v", calls, err)
		}
	}
}

func TestDecisionSummaryCountsEachConsultationOnce(t *testing.T) {
	env := setupRateStore(t)
	ws, ctx := env.seedWorkspace(context.Background(), t)
	recordDecisionOutcomes(ctx, t, NewCallMeter(env.dbFor(ws)))

	now := time.Now().UTC()
	got, err := NewCallReadStore(env.dbFor(ws)).DecisionSummaries(diagnosticsReader(ws), now.Add(-time.Hour), now)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("summaries = %+v, want site triage alone", got)
	}
	triage := got[0]
	if triage.Task != string(TaskSiteTriage) || triage.Asked != 4 || triage.Decided != 1 {
		t.Errorf("triage = %+v, want asked 4, decided 1", triage)
	}
	want := map[string]int{
		attemptReasonDecisionBelowFloor: 1, attemptReasonDecisionError: 1, attemptReasonDecisionUncertified: 1,
	}
	if !maps.Equal(triage.Fallbacks, want) {
		t.Errorf("fallbacks = %v, want %v", triage.Fallbacks, want)
	}
}

func TestDecisionSummaryOfAnEmptyWindowIsEmpty(t *testing.T) {
	env := setupRateStore(t)
	ws, _ := env.seedWorkspace(context.Background(), t)
	now := time.Now().UTC()
	got, err := NewCallReadStore(env.dbFor(ws)).DecisionSummaries(diagnosticsReader(ws), now.Add(-time.Hour), now)
	if err != nil || len(got) != 0 {
		t.Fatalf("summaries = %+v, %v; want none", got, err)
	}
}

func TestDecisionSummaryRefusesAReaderWithoutTheDiagnosticsGrant(t *testing.T) {
	env := setupRateStore(t)
	ws, ctx := env.seedWorkspace(context.Background(), t)
	recordDecisionOutcomes(ctx, t, NewCallMeter(env.dbFor(ws)))

	stranger := principal.WithActor(principal.WithWorkspaceID(context.Background(), ws), principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:no-diagnostics",
		Permissions: principal.Permissions{RoleKeys: []string{"fixture"}},
	})
	now := time.Now().UTC()
	_, err := NewCallReadStore(env.dbFor(ws)).DecisionSummaries(stranger, now.Add(-time.Hour), now)
	if !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Fatalf("err = %v, want permission denied", err)
	}
}

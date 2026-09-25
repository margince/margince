// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package ai

// The rung health read against the real ai_call table, every logical call
// written through the router's own recorder in the shapes the ladder leaves.

import (
	"context"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// ladderRow is one chat-ladder attempt as the router records it.
func ladderRow(logical ids.UUID, attempt int, terminal bool, tier Tier, reason, sentinel string) Call {
	return Call{
		LogicalCallID: logical, Attempt: attempt, IsTerminal: terminal, Kind: callKindCompletion,
		Task: TaskSiteTriage, Tier: tier, Provider: "test", ModelID: "test-model", RequestFingerprint: "fp-health",
		AttemptReason: reason, ErrorSentinel: sentinel, TokensIn: 100, TokensOut: 10,
	}
}

// rungHealthAfter records each logical call through CallMeter.Record and reads
// the health report back, keyed by tier.
func rungHealthAfter(t *testing.T, logicalCalls ...[]Call) map[string]RungHealth {
	t.Helper()
	env := setupRateStore(t)
	ws, ctx := env.seedWorkspace(context.Background(), t)
	calls := NewCallMeter(env.dbFor(ws))
	for _, attempts := range logicalCalls {
		if err := calls.Record(ctx, attempts); err != nil {
			t.Fatalf("recording %+v: %v", attempts, err)
		}
	}
	report, err := NewMeter(env.dbFor(ws)).RungHealthReport(diagnosticsReader(ws))
	if err != nil {
		t.Fatal(err)
	}
	byTier := map[string]RungHealth{}
	for _, rung := range report {
		byTier[rung.Tier] = rung
	}
	return byTier
}

// A tier that fails and hands the call to the next one failed that call, even
// though the caller got an answer. Counted only on terminal attempts, a tier
// failing over on every call reads "N calls, 0 failed".
func TestRungHealthCountsATierThatFailedOverToTheNext(t *testing.T) {
	logical := ids.NewV7()
	rungs := rungHealthAfter(t, []Call{
		ladderRow(logical, 1, false, TierLocalSmall, "", "provider_unavailable"),
		ladderRow(logical, 2, true, TierCheapCloud, attemptReasonProviderError, ""),
	})

	failed := rungs[string(TierLocalSmall)]
	if failed.Calls != 1 || failed.Failures != 1 {
		t.Errorf("%s calls/failures = %d/%d, want 1/1", TierLocalSmall, failed.Calls, failed.Failures)
	}
	if failed.Healthy() || failed.LastSentinel != "provider_unavailable" {
		t.Errorf("%s = %+v, want unhealthy with its sentinel", TierLocalSmall, failed)
	}
	answered := rungs[string(TierCheapCloud)]
	if answered.Calls != 1 || answered.Failures != 0 || !answered.Healthy() {
		t.Errorf("%s = %+v, want 1 call, 0 failed, healthy", TierCheapCloud, answered)
	}
}

// A schema-invalid retry is a second attempt on the same tier, and the first
// attempt ANSWERED: the provider replied and the task's validator refused the
// text. Both count as calls and neither as a failure — this surface asks
// whether the tier responds, not whether its answers pass a task's check.
func TestRungHealthCountsASchemaRetryAsAnotherAnsweredCall(t *testing.T) {
	logical := ids.NewV7()
	rungs := rungHealthAfter(t, []Call{
		ladderRow(logical, 1, false, TierCheapCloud, "", ""),
		ladderRow(logical, 2, true, TierCheapCloud, attemptReasonSchemaInvalid, ""),
	})

	rung := rungs[string(TierCheapCloud)]
	if rung.Calls != 2 || rung.Failures != 0 || !rung.Healthy() {
		t.Errorf("%s = %+v, want 2 calls, 0 failed, healthy", TierCheapCloud, rung)
	}
}

// Every attempt of one logical call is written in one transaction, so they
// share occurred_at. The tier's latest outcome is then its highest attempt:
// here it answered first and failed on the retry, and it is down now.
func TestRungHealthReadsATiersLatestAttemptWithinOneCall(t *testing.T) {
	logical := ids.NewV7()
	rungs := rungHealthAfter(t, []Call{
		ladderRow(logical, 1, false, TierLocalSmall, "", ""),
		ladderRow(logical, 2, false, TierLocalSmall, attemptReasonProviderError, "provider_unavailable"),
		ladderRow(logical, 3, true, TierCheapCloud, attemptReasonProviderError, ""),
	})

	rung := rungs[string(TierLocalSmall)]
	if rung.Calls != 2 || rung.Failures != 1 {
		t.Errorf("%s calls/failures = %d/%d, want 2/1", TierLocalSmall, rung.Calls, rung.Failures)
	}
	if rung.Healthy() {
		t.Errorf("%s = %+v, want unhealthy — its latest attempt failed", TierLocalSmall, rung)
	}
}

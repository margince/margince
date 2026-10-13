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
// the health report back, keyed by tier, with no lane's binding known.
func rungHealthAfter(t *testing.T, logicalCalls ...[]Call) map[string]RungHealth {
	t.Helper()
	return rungHealthBoundTo(t, nil, logicalCalls...)
}

// rungHealthBoundTo is rungHealthAfter with each tier in bound serving the
// model bound names.
func rungHealthBoundTo(t *testing.T, bound map[Tier]ModelRef, logicalCalls ...[]Call) map[string]RungHealth {
	t.Helper()
	env := setupRateStore(t)
	ws, ctx := env.seedWorkspace(context.Background(), t)
	calls := NewCallMeter(env.dbFor(ws))
	for _, attempts := range logicalCalls {
		if err := calls.Record(ctx, attempts); err != nil {
			t.Fatalf("recording %+v: %v", attempts, err)
		}
	}
	report, err := NewMeter(env.dbFor(ws)).RungHealthReport(diagnosticsReader(ws), bound)
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
// here it answered first and failed on the schema retry, and it is down now.
func TestRungHealthReadsATiersLatestAttemptWithinOneCall(t *testing.T) {
	logical := ids.NewV7()
	rungs := rungHealthAfter(t, []Call{
		ladderRow(logical, 1, false, TierLocalSmall, "", ""),
		ladderRow(logical, 2, false, TierLocalSmall, attemptReasonSchemaInvalid, "provider_unavailable"),
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

// A withheld answer or a rejected request is an outcome, not an outage: the
// model was reached and decided. Counted as failures they would mark a
// responding tier down on the one screen an operator reads to find an outage.
func TestRungHealthDoesNotCountAnOutcomeAsAFailure(t *testing.T) {
	withheld, rejected := ids.NewV7(), ids.NewV7()
	rungs := rungHealthAfter(t,
		[]Call{
			ladderRow(withheld, 1, false, TierCheapCloud, "", sentinelOutputWithheld),
			ladderRow(withheld, 2, true, TierPremium, attemptReasonProviderError, ""),
		},
		[]Call{ladderRow(rejected, 1, true, TierCheapCloud, "", sentinelRequestRejected)},
	)
	cheap := rungs[string(TierCheapCloud)]
	if cheap.Calls != 2 || cheap.Failures != 0 || !cheap.Healthy() {
		t.Errorf("%s = %+v, want 2 calls, 0 failed, healthy", TierCheapCloud, cheap)
	}
}

// Two calls committed together share occurred_at, and each is its own attempt
// 1, so neither is "later": the row id breaks the tie, which makes the answer
// arbitrary between them but the SAME answer on every read, so a health badge
// cannot flicker between two reads of one set of rows.
func TestRungHealthAnswersATiedLatestAttemptTheSameOnEveryRead(t *testing.T) {
	env := setupRateStore(t)
	ws, ctx := env.seedWorkspace(context.Background(), t)
	calls := NewCallMeter(env.dbFor(ws))
	failed := ladderRow(ids.NewV7(), 1, true, TierCheapCloud, "", "provider_unavailable")
	answered := ladderRow(ids.NewV7(), 1, true, TierCheapCloud, "", "")
	if err := calls.Record(ctx, []Call{failed, answered}); err != nil {
		t.Fatalf("recording: %v", err)
	}
	var first *RungHealth
	for range 5 {
		report, err := NewMeter(env.dbFor(ws)).RungHealthReport(diagnosticsReader(ws), nil)
		if err != nil {
			t.Fatal(err)
		}
		for _, rung := range report {
			if rung.Tier != string(TierCheapCloud) {
				continue
			}
			if rung.Calls != 2 || rung.Failures != 1 {
				t.Fatalf("%s calls/failures = %d/%d, want 2/1", TierCheapCloud, rung.Calls, rung.Failures)
			}
			if first == nil {
				first = &rung
			} else if rung.Healthy() != first.Healthy() || rung.LastSentinel != first.LastSentinel {
				t.Fatalf("two reads of the same rows disagree: %+v then %+v", *first, rung)
			}
		}
	}
	if first == nil {
		t.Fatalf("no %s rung in the report", TierCheapCloud)
	}
}

// madeBy is one answered or failed attempt on tier by provider's model.
func madeBy(tier Tier, provider, modelID, sentinel string) []Call {
	row := ladderRow(ids.NewV7(), 1, true, tier, "", sentinel)
	row.Provider, row.ModelID = provider, modelID
	return []Call{row}
}

// A lane rebound to another model reads as that model, which has made no call.
// The previous model's failures are not the new one's. Shown on its row, they
// would send an admin to replace a model that works.
func TestRungHealthDropsTheAttemptsOfTheModelATierWasBoundToBefore(t *testing.T) {
	bound := map[Tier]ModelRef{
		TierPremium:   {Provider: "gemini", Model: "gemini-3.5-flash"},
		TierEmbedLane: {Provider: "gemini", Model: "text-embedding-005"},
	}
	rungs := rungHealthBoundTo(t, bound,
		madeBy(TierPremium, "openai_compatible", "gpt-5.4-mini", "provider_error"),
		madeBy(TierEmbedLane, "openai", "text-embedding-3-small", "provider_error"),
	)
	for _, tier := range []Tier{TierPremium, TierEmbedLane} {
		if rung, ok := rungs[string(tier)]; ok {
			t.Errorf("%s = %+v, want no row: every attempt was made by a model it is no longer bound to", tier, rung)
		}
	}
}

// Only the previous model's attempts leave the row. The bound model's own keep
// counting, and a tier rebound to the model it already had keeps its history.
// A tier the binding does not name keeps every attempt it made.
func TestRungHealthKeepsTheAttemptsOfTheModelATierIsBoundToNow(t *testing.T) {
	bound := map[Tier]ModelRef{
		TierPremium:    {Provider: "gemini", Model: "gemini-3.5-flash"},
		TierCheapCloud: {Provider: "test", Model: "test-model"},
	}
	rungs := rungHealthBoundTo(t, bound,
		madeBy(TierPremium, "openai_compatible", "gpt-5.4-mini", "provider_error"),
		madeBy(TierPremium, "gemini", "gemini-3.5-flash", ""),
		madeBy(TierCheapCloud, "test", "test-model", "provider_unavailable"),
		madeBy(TierLocalSmall, "ollama", "retired-model", "provider_unavailable"),
	)
	premium := rungs[string(TierPremium)]
	if premium.Calls != 1 || premium.Failures != 0 || !premium.Healthy() || premium.LastSentinel != "" {
		t.Errorf("%s = %+v, want the bound model's 1 answered call alone", TierPremium, premium)
	}
	cheap := rungs[string(TierCheapCloud)]
	if cheap.Calls != 1 || cheap.Failures != 1 || cheap.LastSentinel != "provider_unavailable" {
		t.Errorf("%s = %+v, want its bound model's failure kept", TierCheapCloud, cheap)
	}
	unnamed := rungs[string(TierLocalSmall)]
	if unnamed.Calls != 1 || unnamed.Failures != 1 {
		t.Errorf("%s = %+v, want every attempt of a tier the binding does not name", TierLocalSmall, unnamed)
	}
}

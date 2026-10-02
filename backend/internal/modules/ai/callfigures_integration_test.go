// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package ai

// The call figures an admin tunes by, over rows the router's own writer
// recorded: a decision task whose decision model answered most calls, timed
// out on some, and whose fallback failed once.

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

type figuresFixture struct {
	reader *CallReadStore
	ctx    context.Context
	task   Task
}

// seedFigures records 10 logical calls of one decision task: 8 the decision
// model answered, 2 it timed out on and the cheap tier picked up — one
// answered, one failed — plus a cache hit, which no figure counts.
func seedFigures(t *testing.T) figuresFixture {
	t.Helper()
	env := setupRateStore(t)
	ctx := context.Background()
	ws, ctx := env.seedWorkspace(ctx, t)
	db := env.dbFor(ws)
	meter := NewCallMeter(db)
	task := Task("figures_" + ids.NewV7().String()[:8])
	env.insertRate(ctx, t, ModelRate{Provider: providerOpenAICompatible, ModelID: "openai/gpt-oss-120b",
		InputPerMTokMicroUSD: 1_000_000, OutputPerMTokMicroUSD: 2_000_000, EffectiveDate: time.Now().AddDate(0, 0, -1)})
	decision := func(sentinel string, terminal bool) Call {
		return Call{Kind: callKindDecision, Task: task, Tier: TierDecideLane, Provider: providerJevCompatible, ModelID: "typesafe/jev-1.13",
			RequestFingerprint: "fp-" + ids.NewV7().String(), ErrorSentinel: sentinel, IsTerminal: terminal, LatencyMS: 900}
	}
	ladder := func(sentinel, reason string) Call {
		c := Call{Kind: callKindCompletion, Task: task, Tier: TierCheapCloud, Provider: providerOpenAICompatible, ModelID: "openai/gpt-oss-120b",
			RequestFingerprint: "fp-" + ids.NewV7().String(), ErrorSentinel: sentinel, AttemptReason: reason, Attempt: 2, IsTerminal: true, LatencyMS: 4000}
		if sentinel == "" {
			c.TokensIn, c.TokensOut = 1000, 500
		}
		return c
	}
	var calls [][]Call
	for range 8 {
		calls = append(calls, []Call{decision("", true)})
	}
	calls = append(calls,
		[]Call{decision(sentinelTimeout, false), ladder("", attemptReasonDecisionError)},
		[]Call{decision(sentinelTimeout, false), ladder("provider_error", attemptReasonDecisionError)},
	)
	for _, attempts := range calls {
		logical := ids.NewV7()
		for i := range attempts {
			attempts[i].LogicalCallID = logical
			if attempts[i].Attempt == 0 {
				attempts[i].Attempt = 1
			}
		}
		if err := meter.Record(ctx, attempts); err != nil {
			t.Fatal(err)
		}
	}
	if err := meter.Record(ctx, []Call{{LogicalCallID: ids.NewV7(), Attempt: 1, IsTerminal: true, Kind: callKindCompletion, Task: task,
		Tier: TierCheapCloud, Provider: providerOpenAICompatible, ModelID: "openai/gpt-oss-120b", RequestFingerprint: "fp-cached", CacheHit: true}}); err != nil {
		t.Fatal(err)
	}
	return figuresFixture{reader: NewCallReadStore(db), ctx: diagnosticsReader(ws), task: task}
}

func TestCallStatsCountsFailuresTimeoutsAndCost(t *testing.T) {
	f := seedFigures(t)

	rows, err := f.reader.CallStats(f.ctx, CallStatsQuery{Window: 7 * 24 * time.Hour, GroupBy: GroupByProvider, Filter: CallStatsFilter{Task: f.task}})

	if err != nil {
		t.Fatal(err)
	}
	byKey := map[string]CallStatsRow{}
	for _, row := range rows {
		byKey[row.Key] = row
	}
	decide, broker := byKey[providerJevCompatible], byKey[providerOpenAICompatible]
	if decide.Calls != 10 || decide.Failed != 2 || decide.Timeouts != 2 {
		t.Errorf("decision figures = %+v, want 10 calls, 2 failed, both timeouts", decide)
	}
	if broker.Calls != 2 || broker.Failed != 1 || broker.Timeouts != 0 || broker.P50Ms != 4000 {
		t.Errorf("broker figures = %+v, want 2 calls, 1 failure, the cache hit not counted", broker)
	}
	// 1000 in at $1/Mtok and 500 out at $2/Mtok is 2000 micro-USD.
	if broker.CostMicroUSD != 2000 || broker.Unpriced != 0 {
		t.Errorf("broker cost = %d µUSD (%d unpriced), want 2000", broker.CostMicroUSD, broker.Unpriced)
	}
}

func TestTaskFlowCountsTheUnansweredLogicalCall(t *testing.T) {
	f := seedFigures(t)

	flow, err := f.reader.TaskFlow(f.ctx, f.task, 7*24*time.Hour)

	if err != nil {
		t.Fatal(err)
	}
	if flow.Total != 10 || flow.Unanswered != 1 || len(flow.Steps) != 2 {
		t.Fatalf("flow = %+v, want 10 calls, 1 unanswered, two steps", flow)
	}
	decide, cheap := flow.Steps[0], flow.Steps[1]
	if !decide.Decision || decide.Attempts != 10 || decide.Answered != 8 || decide.GaveUp[sentinelTimeout] != 2 {
		t.Errorf("decision step = %+v", decide)
	}
	if cheap.Tier != TierCheapCloud || cheap.Answered != 1 || cheap.GaveUp["provider_error"] != 1 {
		t.Errorf("fallback step = %+v", cheap)
	}
}

func TestAnEmptyWindowReturnsNoRows(t *testing.T) {
	f := seedFigures(t)
	f.reader.now = func() time.Time { return time.Now().Add(40 * 24 * time.Hour) }

	rows, err := f.reader.CallStats(f.ctx, CallStatsQuery{Window: 30 * 24 * time.Hour, GroupBy: GroupByTask})
	if err != nil || rows == nil || len(rows) != 0 {
		t.Fatalf("rows = %+v (%v), want an empty list", rows, err)
	}
	flow, err := f.reader.TaskFlow(f.ctx, f.task, 30*24*time.Hour)
	if err != nil || flow.Total != 0 || flow.Unanswered != 0 || len(flow.Steps) != 0 {
		t.Fatalf("flow = %+v (%v), want nothing", flow, err)
	}
}

func TestTheCallListNarrowsToOneProviderAndTier(t *testing.T) {
	f := seedFigures(t)

	page, err := f.reader.ListCalls(f.ctx, nil, nil, CallListFilter{Task: string(f.task), Provider: providerOpenAICompatible, Tier: string(TierCheapCloud)})

	if err != nil {
		t.Fatal(err)
	}
	// The cache hit and both fallbacks ended on the cheap tier.
	if len(page.Items) != 3 {
		t.Fatalf("listed %d calls, want the 3 that ended on the broker", len(page.Items))
	}
	for _, item := range page.Items {
		if item.Provider != providerOpenAICompatible {
			t.Errorf("listed a call on %s", item.Provider)
		}
	}
}

func TestAnUnknownGroupingIsRefused(t *testing.T) {
	f := seedFigures(t)
	var faults routingFaults
	_, err := f.reader.CallStats(f.ctx, CallStatsQuery{Window: time.Hour, GroupBy: "colour"})
	if !errors.As(err, &faults) || faults[0].Path != "group" {
		t.Fatalf("err = %v", err)
	}
}

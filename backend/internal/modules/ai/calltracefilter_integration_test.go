// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package ai

// The call trace narrows on the server and prices each call the way the usage
// report does. Every call is written through CallMeter, the writer production
// uses, so the filters are proven against the rows the router really leaves.

import (
	"context"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

const (
	tracePricedModel   = "trace-priced"
	traceUnpricedModel = "trace-unpriced"
	traceServedModel   = "trace-served-snapshot"
)

// traceCall is a first-attempt terminal completion; a case edits what differs.
func traceCall(sentinel string, tokensIn, tokensOut int, model string) Call {
	return Call{
		Attempt: 1, IsTerminal: true, Kind: callKindCompletion,
		Task: TaskSummarize, Tier: TierCheapCloud, Provider: providerOpenAICompatible,
		ModelID: model, RequestFingerprint: "fp-" + ids.NewV7().String(),
		TokensIn: tokensIn, TokensOut: tokensOut, ErrorSentinel: sentinel,
	}
}

func recordLogical(ctx context.Context, t *testing.T, meter *CallMeter, attempts ...Call) {
	t.Helper()
	logical := ids.NewV7()
	for i := range attempts {
		attempts[i].LogicalCallID = logical
	}
	if err := meter.Record(ctx, attempts); err != nil {
		t.Fatalf("recording a logical call: %v", err)
	}
}

// databaseNow is the clock ai_call.occurred_at is stamped with. A rate or report
// window dated from it meets the seeded rows however the lane moved time.
func (e *rateEnv) databaseNow(ctx context.Context, t *testing.T) time.Time {
	t.Helper()
	var at time.Time
	if err := e.owner.QueryRow(ctx, `SELECT now()`).Scan(&at); err != nil {
		t.Fatalf("reading the database clock: %v", err)
	}
	return at.UTC()
}

// seedTrace writes one call per outcome bucket plus one the price sheet cannot
// price. Only the answered call reports a served model.
func seedTrace(ctx context.Context, t *testing.T, env *rateEnv, meter *CallMeter) {
	t.Helper()
	today := env.databaseNow(ctx, t).Truncate(24 * time.Hour)
	env.insertRate(ctx, t, ModelRate{
		Provider: providerOpenAICompatible, ModelID: tracePricedModel,
		InputPerMTokMicroUSD: 2_000_000, OutputPerMTokMicroUSD: 8_000_000,
		EffectiveDate: today.AddDate(0, 0, -1),
	})

	answered := traceCall("", 1000, 500, tracePricedModel)
	answered.ServedModel = traceServedModel
	recordLogical(ctx, t, meter, answered)

	retried := traceCall("provider_error", 200, 0, tracePricedModel)
	retried.IsTerminal = false
	fallback := traceCall("", 300, 100, tracePricedModel)
	fallback.Attempt, fallback.AttemptReason = 2, "provider_error"
	recordLogical(ctx, t, meter, retried, fallback)

	recordLogical(ctx, t, meter, traceCall(sentinelOutputWithheld, 100, 0, tracePricedModel))
	recordLogical(ctx, t, meter, traceCall("provider_quota", 0, 0, tracePricedModel))
	recordLogical(ctx, t, meter, traceCall("", 10, 10, traceUnpricedModel))
}

func listTrace(reader context.Context, t *testing.T, store *CallReadStore, filter CallListFilter) []CallSummary {
	t.Helper()
	page, err := store.ListCalls(reader, nil, nil, filter)
	if err != nil {
		t.Fatalf("listing calls under %+v: %v", filter, err)
	}
	return page.Items
}

func TestTheCallTraceBucketsAndPricesEachCall(t *testing.T) {
	env := setupRateStore(t)
	ctx := context.Background()
	ws, ctx := env.seedWorkspace(ctx, t)
	db := env.dbFor(ws)
	seedTrace(ctx, t, env, NewCallMeter(db))
	store, reader := NewCallReadStore(db), diagnosticsReader(ws)

	// The retried call costs both of its attempts: 200 in at $2/MTok plus
	// 300 in and 100 out at $2 and $8, in micro-USD.
	wantCost := map[string]int64{
		"answered": 2000 + 4000,
		"fallback": 400 + 600 + 800,
		"declined": 200,
		"failed":   0,
	}
	for outcome, want := range wantCost {
		got := listTrace(reader, t, store, CallListFilter{Outcome: outcome, Model: tracePricedModel})
		if outcome == "answered" {
			got = listTrace(reader, t, store, CallListFilter{Outcome: outcome, Model: traceServedModel})
		}
		if len(got) != 1 {
			t.Fatalf("outcome %s: %d calls, want exactly the one seeded", outcome, len(got))
		}
		if got[0].Outcome != outcome {
			t.Errorf("outcome %s: the row reports %q", outcome, got[0].Outcome)
		}
		if got[0].CostMicroUSD == nil || *got[0].CostMicroUSD != want {
			t.Errorf("outcome %s: cost %v, want %d", outcome, got[0].CostMicroUSD, want)
		}
	}

	unpriced := listTrace(reader, t, store, CallListFilter{Model: traceUnpricedModel})
	if len(unpriced) != 1 || unpriced[0].CostMicroUSD != nil {
		t.Fatalf("a call with no rate must carry no cost, never a 0: %+v", unpriced)
	}

	// The per-call costs are the usage report's own numbers: they sum to it.
	all := listTrace(reader, t, store, CallListFilter{})
	var listed int64
	for _, call := range all {
		if call.CostMicroUSD != nil {
			listed += *call.CostMicroUSD
		}
	}
	now := env.databaseNow(ctx, t)
	report, err := NewRateStore(db).CostReport(ctx, now.Add(-time.Hour), now.Add(time.Hour))
	if err != nil {
		t.Fatalf("cost report: %v", err)
	}
	var reported int64
	for _, line := range report {
		reported += line.CostMicroUSD
	}
	if listed != reported {
		t.Errorf("the trace's costs sum to %d micro-USD, the usage report to %d", listed, reported)
	}
}

func TestTheCallTraceNarrowsOnTheServer(t *testing.T) {
	env := setupRateStore(t)
	ctx := context.Background()
	ws, ctx := env.seedWorkspace(ctx, t)
	db := env.dbFor(ws)
	seedTrace(ctx, t, env, NewCallMeter(db))
	store, reader := NewCallReadStore(db), diagnosticsReader(ws)

	all := listTrace(reader, t, store, CallListFilter{})
	if len(all) != 5 {
		t.Fatalf("trace lists %d terminal calls, want the 5 seeded", len(all))
	}
	page, err := store.ListCalls(reader, nil, nil, CallListFilter{Outcome: "failed"})
	if err != nil {
		t.Fatal(err)
	}
	wantModels := []string{traceServedModel, tracePricedModel, traceUnpricedModel}
	assertSetEqual(t, "model facets under a filter", page.Models, wantModels)

	cases := map[string]struct {
		filter CallListFilter
		want   int
	}{
		"search by error code":         {CallListFilter{Query: "QUOTA"}, 1},
		"search by task in words":      {CallListFilter{Query: "summar"}, 5},
		"search treats % as a literal": {CallListFilter{Query: "%"}, 0},
		"model and outcome combine":    {CallListFilter{Model: traceServedModel, Outcome: "failed"}, 0},
		"configured model":             {CallListFilter{Model: tracePricedModel}, 4},
		"served model":                 {CallListFilter{Model: traceServedModel}, 1},
		"task that ran nothing":        {CallListFilter{Task: string(TaskEnrich)}, 0},
	}
	for name, tc := range cases {
		if got := listTrace(reader, t, store, tc.filter); len(got) != tc.want {
			t.Errorf("%s: %d calls, want %d", name, len(got), tc.want)
		}
	}

	// The trace is newest first, so the oldest and newest bound the window.
	newest, oldest := all[0].OccurredAt, all[len(all)-1].OccurredAt
	if !oldest.Before(newest) {
		t.Fatalf("seeded calls share one instant (%s); the window cannot be told apart", oldest)
	}
	if got := listTrace(reader, t, store, CallListFilter{From: &newest}); len(got) == 0 || got[0].ID != all[0].ID {
		t.Errorf("from the newest instant lists %d calls, want it first", len(got))
	}
	if got := listTrace(reader, t, store, CallListFilter{To: &newest}); len(got) != len(all)-countAt(all, newest) {
		t.Errorf("to is exclusive: %d calls before the newest, want %d", len(got), len(all)-countAt(all, newest))
	}
}

func countAt(calls []CallSummary, at time.Time) int {
	n := 0
	for _, call := range calls {
		if call.OccurredAt.Equal(at) {
			n++
		}
	}
	return n
}

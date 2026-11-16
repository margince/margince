// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package ai

// What the call trace records about WHO served a completion.
//
// These columns are written by production and read by nothing in production —
// they are audit trail, which is a legitimate shape for a column and an awkward
// one to test, because there is no reader to assert through. So the read here is
// direct SQL. The WRITE is emphatically not: it goes through CallMeter, the same
// path the router uses, because a test that inserted the row itself would prove
// only that Postgres stores text.

import (
	"context"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// A broker's upstream survives the real write path.
func TestTheCallTraceRecordsWhichUpstreamServed(t *testing.T) {
	t.Parallel()
	env := setupRateStore(t)
	ctx := context.Background()
	ws, ctx := env.seedWorkspace(ctx, t)
	meter := NewCallMeter(env.dbFor(ws))

	logical := ids.NewV7()
	if err := meter.Record(ctx, []Call{{
		LogicalCallID: logical, Attempt: 1, IsTerminal: true,
		Kind: callKindCompletion, Task: TaskSummarize, Tier: TierCheapCloud,
		Provider: providerOpenAICompatible, ModelID: "openai/gpt-oss-120b",
		// The echoed model and the named upstream disagree on purpose: that is
		// the broker case, and the reason these are two columns.
		ServedModel: "openai/gpt-oss-120b", ServedIdentitySource: servedIdentitySourceEcho,
		ServedProvider: "BaseTen",
	}}); err != nil {
		t.Fatalf("recording the call: %v", err)
	}

	var provider, source string
	if err := env.owner.QueryRow(
		ctx, `
		SELECT served_provider, served_identity_source
		  FROM ai_call WHERE logical_call_id = $1`, logical,
	).Scan(&provider, &source); err != nil {
		t.Fatalf("reading the call back: %v", err)
	}
	if provider != "BaseTen" {
		t.Errorf("served_provider = %q, want BaseTen", provider)
	}
	// Naming the upstream does NOT promote the echoed model to a confirmation.
	// We learned who served, not what they served, and collapsing the two would
	// launder an echo into a report.
	if source != servedIdentitySourceEcho {
		t.Errorf("served_identity_source = %q, want %q", source, servedIdentitySourceEcho)
	}
}

// A direct vendor names no upstream, and the column stays empty rather than
// being backfilled with the provider we called — the distinction the whole
// column exists to draw.
func TestACallWithNoReportedUpstreamStoresNoUpstream(t *testing.T) {
	t.Parallel()
	env := setupRateStore(t)
	ctx := context.Background()
	ws, ctx := env.seedWorkspace(ctx, t)
	meter := NewCallMeter(env.dbFor(ws))

	logical := ids.NewV7()
	if err := meter.Record(ctx, []Call{{
		LogicalCallID: logical, Attempt: 1, IsTerminal: true,
		Kind: callKindCompletion, Task: TaskSummarize, Tier: TierPremium,
		Provider: providerGemini, ModelID: "gemini-3.5-flash",
		ServedModel:          "gemini-3.5-flash",
		ServedIdentitySource: servedIdentitySourceResponse,
	}}); err != nil {
		t.Fatalf("recording the call: %v", err)
	}

	var provider string
	if err := env.owner.QueryRow(
		ctx, `
		SELECT served_provider FROM ai_call WHERE logical_call_id = $1`, logical,
	).Scan(&provider); err != nil {
		t.Fatalf("reading the call back: %v", err)
	}
	if provider != "" {
		t.Errorf("served_provider = %q, want it empty", provider)
	}
}

// An absent error sentinel is NULL, not the empty string.
//
// Worth a test of its own because it used to be SQL's job: the statement wrapped
// the placeholder in NULLIF, and deriving the placeholders moved that decision
// into Go. A regression here is invisible — ” and NULL both read as "no error"
// to a human skimming the column, and only a query filtering IS NULL notices.
func TestACallWithoutAnErrorStoresNullRatherThanEmptyText(t *testing.T) {
	t.Parallel()
	env := setupRateStore(t)
	ctx := context.Background()
	ws, ctx := env.seedWorkspace(ctx, t)
	meter := NewCallMeter(env.dbFor(ws))

	for _, tc := range []struct {
		name     string
		sentinel string
		wantNull bool
	}{
		{"no error", "", true},
		{"an error", "provider_error", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			logical := ids.NewV7()
			if err := meter.Record(ctx, []Call{{
				LogicalCallID: logical, Attempt: 1, IsTerminal: true,
				Kind: callKindCompletion, Task: TaskSummarize, Tier: TierCheapCloud,
				Provider: providerGemini, ModelID: "m",
				ErrorSentinel: tc.sentinel,
			}}); err != nil {
				t.Fatalf("recording the call: %v", err)
			}
			var isNull bool
			if err := env.owner.QueryRow(
				ctx, `
				SELECT error_sentinel IS NULL FROM ai_call WHERE logical_call_id = $1`, logical,
			).Scan(&isNull); err != nil {
				t.Fatalf("reading the call back: %v", err)
			}
			if isNull != tc.wantNull {
				t.Errorf("error_sentinel IS NULL = %v, want %v", isNull, tc.wantNull)
			}
		})
	}
}

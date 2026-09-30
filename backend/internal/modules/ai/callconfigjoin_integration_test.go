// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package ai

// A call's configuration hash resolves to what it names.
//
// `ai_call_config` is the dimension `ai_call.config_hash` points at, and until
// this join nothing read it: a diagnostics reader could see that two calls
// shared a configuration and never what it was, or what changed between two
// that did not. `EnsureConfig` paid for the dimension on every call and
// answered nothing.
//
// Both rows are written through the real writers — EnsureConfig plants the
// snapshot and CallMeter records the call — because a test that inserted them
// itself would prove only that Postgres stores text.

import (
	"context"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestACallResolvesTheConfigurationItRanUnder(t *testing.T) {
	env := setupRateStore(t)
	ctx := context.Background()
	ws, ctx := env.seedWorkspace(ctx, t)
	db := env.dbFor(ws)
	meter := NewCallMeter(db)

	snap := ConfigSnapshot{
		Hash:              "cfg-" + ids.NewV7().String(),
		TaskContractHash:  "task-contract-v3",
		RoutingConfigHash: "routing-v7",
		PromptVersion:     "summarize@v7",
		ProviderParams:    []byte(`{"temperature":0.2}`),
	}
	if err := meter.EnsureConfig(ctx, snap); err != nil {
		t.Fatalf("planting the configuration snapshot: %v", err)
	}

	logical := ids.NewV7()
	if err := meter.Record(ctx, []Call{{
		LogicalCallID: logical, Attempt: 1, IsTerminal: true,
		Kind: callKindCompletion, Task: TaskSummarize, Tier: TierCheapCloud,
		Provider: providerOpenAICompatible, ModelID: "openai/gpt-oss-120b",
		RequestFingerprint: "fp-config-join", ConfigHash: &snap.Hash,
	}}); err != nil {
		t.Fatalf("recording the call: %v", err)
	}

	reader := diagnosticsReader(ws)
	page, err := NewCallReadStore(db).ListCalls(reader, nil, nil, nil)
	if err != nil {
		t.Fatalf("listing calls: %v", err)
	}
	if len(page.Items) == 0 {
		t.Fatal("the call this case recorded is not in the trace")
	}
	detail, err := NewCallReadStore(db).GetCall(reader, page.Items[0].ID)
	if err != nil {
		t.Fatalf("reading the call back: %v", err)
	}

	if detail.Config == nil {
		t.Fatal("a call whose configuration was planted resolves to nothing — the hash still " +
			"says only that two calls agreed, which is the question this dimension exists to answer")
	}
	if got := detail.Config.PromptVersion; got != snap.PromptVersion {
		t.Errorf("prompt version reads %q, want %q", got, snap.PromptVersion)
	}
	if got := detail.Config.RoutingConfigHash; got != snap.RoutingConfigHash {
		t.Errorf("routing config reads %q, want %q", got, snap.RoutingConfigHash)
	}
	if detail.Config.ProviderParams == nil {
		t.Fatal("the provider parameters this configuration pinned came back empty")
	}
	if got := (*detail.Config.ProviderParams)["temperature"]; got != 0.2 {
		t.Errorf("temperature reads %v, want the 0.2 the snapshot pinned", got)
	}
}

// A call that names no configuration resolves to nothing, rather than to a
// configuration that pinned nothing.
//
// This is the ONLY way the join misses. ai_call_config_fk makes config_hash a
// foreign key into the dimension, so a call carrying a hash always has a row
// behind it — a call with no hash at all is the whole of the nil case.
func TestACallNamingNoConfigurationResolvesToNothing(t *testing.T) {
	env := setupRateStore(t)
	ctx := context.Background()
	ws, ctx := env.seedWorkspace(ctx, t)
	db := env.dbFor(ws)

	if err := NewCallMeter(db).Record(ctx, []Call{{
		LogicalCallID: ids.NewV7(), Attempt: 1, IsTerminal: true,
		Kind: callKindCompletion, Task: TaskSummarize, Tier: TierCheapCloud,
		Provider: providerOpenAICompatible, ModelID: "openai/gpt-oss-120b",
		RequestFingerprint: "fp-no-config",
	}}); err != nil {
		t.Fatalf("recording the call: %v", err)
	}

	reader := diagnosticsReader(ws)
	page, err := NewCallReadStore(db).ListCalls(reader, nil, nil, nil)
	if err != nil {
		t.Fatalf("listing calls: %v", err)
	}
	detail, err := NewCallReadStore(db).GetCall(reader, page.Items[0].ID)
	if err != nil {
		t.Fatalf("reading the call back: %v", err)
	}
	if detail.Config != nil {
		t.Fatalf("a call naming no configuration resolved to %+v", *detail.Config)
	}
	if detail.ConfigHash != nil {
		t.Errorf("the call reports hash %q, but it was recorded without one", *detail.ConfigHash)
	}
}

// `provider_params` is `jsonb NOT NULL`, and that admits the JSON value `null`:
// EnsureConfig takes the snapshot's raw bytes and inserts them without checking
// the shape. Unmarshalled, a stored null gives a nil map with no error, and
// assigning it would hand a reader a non-nil pointer to nothing — a
// configuration reporting an empty object it never pinned.
func TestAStoredJSONNullIsNotReadAsAnEmptyParameterSet(t *testing.T) {
	env := setupRateStore(t)
	ctx := context.Background()
	ws, ctx := env.seedWorkspace(ctx, t)
	db := env.dbFor(ws)
	meter := NewCallMeter(db)

	snap := ConfigSnapshot{
		Hash:              "cfg-null-" + ids.NewV7().String(),
		TaskContractHash:  "task-contract-v3",
		RoutingConfigHash: "routing-v7",
		PromptVersion:     "summarize@v7",
		ProviderParams:    []byte(`null`),
	}
	if err := meter.EnsureConfig(ctx, snap); err != nil {
		t.Fatalf("planting the configuration snapshot: %v", err)
	}
	if err := meter.Record(ctx, []Call{{
		LogicalCallID: ids.NewV7(), Attempt: 1, IsTerminal: true,
		Kind: callKindCompletion, Task: TaskSummarize, Tier: TierCheapCloud,
		Provider: providerOpenAICompatible, ModelID: "openai/gpt-oss-120b",
		RequestFingerprint: "fp-null-params", ConfigHash: &snap.Hash,
	}}); err != nil {
		t.Fatalf("recording the call: %v", err)
	}

	reader := diagnosticsReader(ws)
	page, err := NewCallReadStore(db).ListCalls(reader, nil, nil, nil)
	if err != nil {
		t.Fatalf("listing calls: %v", err)
	}
	detail, err := NewCallReadStore(db).GetCall(reader, page.Items[0].ID)
	if err != nil {
		t.Fatalf("reading the call back: %v", err)
	}
	if detail.Config == nil {
		t.Fatal("the configuration itself must still resolve — only its parameters were null")
	}
	if detail.Config.ProviderParams != nil {
		t.Fatalf("a stored JSON null read as %+v, want no parameter set at all",
			*detail.Config.ProviderParams)
	}
}

// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package ai

// What a priced model is FOR, against a real sheet. The lane is what the
// routing form reads to offer a chat model where a chat tier binds and an
// embedder where the embeddings lane does, so the one thing that must never
// happen is a re-price quietly re-filing a model — the refresh job re-prices
// every model on the sheet and knows nothing about lanes.

import (
	"context"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// laneWriterCtx is a human holding the sheet's write grants in one workspace.
func laneWriterCtx(ws ids.UUID) context.Context {
	return principal.WithActor(principal.WithWorkspaceID(context.Background(), ws), principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:lane-test",
		Permissions: principal.Permissions{
			RoleKeys: []string{"fixture"},
			Objects: map[string]principal.ObjectGrant{
				"ai_model_rate": {Create: true, Read: true, Update: true},
			},
		},
	})
}

// laneOf reads back what the sheet files one model as, through the same list
// the routing form's picker reads.
func laneOf(ctx context.Context, t *testing.T, s *RateStore, provider, model string) Lane {
	t.Helper()
	rows, err := s.ListLatestModelRates(ctx)
	if err != nil {
		t.Fatalf("ListLatestModelRates: %v", err)
	}
	for _, r := range rows {
		if r.Provider == provider && r.ModelID == model {
			return r.Lane
		}
	}
	t.Fatalf("no sheet row for %s/%s", provider, model)
	return ""
}

func TestModelRateLaneDefaultsToChatForAModelTheSheetHasNeverSeen(t *testing.T) {
	e := setupRateStore(t)
	ws, _ := e.seedWorkspace(context.Background(), t)
	ctx := laneWriterCtx(ws)
	store := e.storeFor(ws)

	row, err := store.SetModelRate(ctx, SetModelRateInput{
		Provider: "anthropic", ModelID: "claude-new-1",
		InputUsd: "3", OutputUsd: "15", CacheReadUsd: "0.3", CacheWriteUsd: "3.75",
	})
	if err != nil {
		t.Fatalf("SetModelRate: %v", err)
	}
	if row.Lane != LaneChat {
		t.Errorf("a brand-new model with no lane named is filed as %q, want %q", row.Lane, LaneChat)
	}
}

func TestModelRateLaneIsNotRewrittenByAReprice(t *testing.T) {
	e := setupRateStore(t)
	ws, _ := e.seedWorkspace(context.Background(), t)
	ctx := laneWriterCtx(ws)
	store := e.storeFor(ws)

	const provider, model = "gemini", "gemini-embedding-001"
	if _, err := store.SetModelRate(ctx, SetModelRateInput{
		Provider: provider, ModelID: model,
		InputUsd: "0.15", OutputUsd: "0", CacheReadUsd: "0", CacheWriteUsd: "0",
		Lane: LaneEmbeddings,
	}); err != nil {
		t.Fatalf("file the embedder: %v", err)
	}

	// A later day's price, named by a caller that says nothing about lanes —
	// which is every caller the refresh job has.
	tomorrow := time.Now().UTC().Truncate(24*time.Hour).AddDate(0, 0, 1)
	repriced, err := store.SetModelRate(ctx, SetModelRateInput{
		Provider: provider, ModelID: model,
		InputUsd: "0.20", OutputUsd: "0", CacheReadUsd: "0", CacheWriteUsd: "0",
		EffectiveDate: tomorrow,
	})
	if err != nil {
		t.Fatalf("reprice: %v", err)
	}
	if repriced.Lane != LaneEmbeddings {
		t.Errorf("repricing an embedder re-filed it as %q — the embeddings picker just lost its option "+
			"and four tier pickers gained one that cannot serve a call", repriced.Lane)
	}
	if got := laneOf(ctx, t, store, provider, model); got != LaneEmbeddings {
		t.Errorf("the sheet head reads lane %q, want %q", got, LaneEmbeddings)
	}
}

func TestModelRateLaneIsCorrectedByNamingIt(t *testing.T) {
	e := setupRateStore(t)
	ws, _ := e.seedWorkspace(context.Background(), t)
	ctx := laneWriterCtx(ws)
	store := e.storeFor(ws)

	const provider, model = "openai_compatible", "baai/bge-m3"
	// Mis-filed first — the state every row already on a deployed sheet is in,
	// since the column defaulted them all to chat.
	if _, err := store.SetModelRate(ctx, SetModelRateInput{
		Provider: provider, ModelID: model,
		InputUsd: "0.01", OutputUsd: "0", CacheReadUsd: "0", CacheWriteUsd: "0",
	}); err != nil {
		t.Fatalf("seed the mis-filed row: %v", err)
	}
	if got := laneOf(ctx, t, store, provider, model); got != LaneChat {
		t.Fatalf("fixture bug: expected the mis-filed row to read %q, got %q", LaneChat, got)
	}

	tomorrow := time.Now().UTC().Truncate(24*time.Hour).AddDate(0, 0, 1)
	corrected, err := store.SetModelRate(ctx, SetModelRateInput{
		Provider: provider, ModelID: model,
		InputUsd: "0.01", OutputUsd: "0", CacheReadUsd: "0", CacheWriteUsd: "0",
		EffectiveDate: tomorrow, Lane: LaneEmbeddings,
	})
	if err != nil {
		t.Fatalf("correct the lane: %v", err)
	}
	if corrected.Lane != LaneEmbeddings {
		t.Errorf("naming the lane left it at %q, want %q — there is no other way to fix a mis-filed model",
			corrected.Lane, LaneEmbeddings)
	}
}

// The seeded sheet a fresh workspace is provisioned with must arrive already
// filed, or the picker on a brand-new installation offers every embedder as a
// chat model on the one screen an admin sees first.
func TestSeededSheetArrivesWithItsLanesFiled(t *testing.T) {
	e := setupRateStore(t)
	ctx := context.Background()
	ws, _ := e.seedWorkspace(ctx, t)

	if _, err := e.owner.Exec(ctx, `SELECT set_config('margince.workspace_id', $1, false)`, ws.String()); err != nil {
		t.Fatalf("set workspace guc: %v", err)
	}
	tx, err := e.owner.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer func() {
		if err := tx.Rollback(ctx); err != nil {
			t.Logf("rollback: %v", err)
		}
	}()
	if err := SeedWorkspaceDefaultsTx(ctx, tx, seedRatesTestDay); err != nil {
		t.Fatalf("SeedWorkspaceDefaultsTx: %v", err)
	}

	var embedders int
	if err := tx.QueryRow(ctx,
		`SELECT count(*) FROM ai_model_rate WHERE lane = $1`, string(LaneEmbeddings)).Scan(&embedders); err != nil {
		t.Fatalf("count embedders: %v", err)
	}
	var want int
	for _, r := range SeedModelRates(seedRatesTestDay) {
		if r.Lane == LaneEmbeddings {
			want++
		}
	}
	if want == 0 {
		t.Fatal("the seed sheet declares no embedding model at all — the embeddings picker would be empty everywhere")
	}
	if embedders != want {
		t.Errorf("the seeded sheet holds %d embedding rows, want %d — the seed writes the lane the sheet declares", embedders, want)
	}
}

// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package ai

import (
	"context"
	"testing"
)

// A base-price edit keeps the band the model carries, and the sheet head shows
// it. Only a write that says so drops it.
func TestABasePriceEditKeepsTheBandUnlessItIsCleared(t *testing.T) {
	e := setupRateStore(t)
	ws, _ := e.seedWorkspace(context.Background(), t)
	ctx := laneWriterCtx(ws)
	store := e.storeFor(ws)
	const provider, model = "gemini", "gemini-band-test"
	band := &ModelRateBand{AboveTokens: 200_000, InputUsd: "4", OutputUsd: "18", CacheReadUsd: "0.4", CacheWriteUsd: "0"}
	price := func(input string, clear bool, set *ModelRateBand) ModelRateRow {
		t.Helper()
		row, err := store.SetModelRate(ctx, SetModelRateInput{
			Provider: provider, ModelID: model, InputUsd: input, OutputUsd: "12", CacheReadUsd: "0.2", CacheWriteUsd: "0",
			LongContext: set, ClearLongContext: clear,
		})
		if err != nil {
			t.Fatalf("SetModelRate: %v", err)
		}
		return row
	}
	price("2", false, band)

	edited := price("2.5", false, nil)

	if !sameBand(edited.LongContext, band) {
		t.Fatalf("a same-day base edit left band %+v, want %+v", edited.LongContext, band)
	}
	head, err := store.ListLatestModelRates(ctx)
	if err != nil {
		t.Fatal(err)
	}
	shown := false
	for _, r := range head {
		shown = shown || (r.ModelID == model && sameBand(r.LongContext, band))
	}
	if !shown {
		t.Error("the sheet head does not show the model's band")
	}
	if cleared := price("2.5", true, nil); cleared.LongContext != nil {
		t.Errorf("a write that clears the band left %+v", cleared.LongContext)
	}
}

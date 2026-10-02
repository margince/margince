// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package ai

import (
	"context"
	"errors"
	"testing"
)

func TestEveryWriterFilesWhereItsPriceCameFrom(t *testing.T) {
	e := setupRateStore(t)
	ws, _ := e.seedWorkspace(context.Background(), t)
	ctx := laneWriterCtx(ws)
	store := e.storeFor(ws)
	price := func(model string, source RateSource) (ModelRateRow, error) {
		return store.SetModelRate(ctx, SetModelRateInput{
			Provider: "openai", ModelID: model, InputUsd: "0.25", OutputUsd: "2",
			CacheReadUsd: "0", CacheWriteUsd: "0", Source: source,
		})
	}
	if row, err := price("gpt-5-mini", ""); err != nil || row.Source != RateSourceManual {
		t.Errorf("a price naming no source = %+v, %v; want manual", row, err)
	}
	if row, err := price("gpt-5-nano", RateSourceCatalogue); err != nil || row.Source != RateSourceCatalogue {
		t.Errorf("a synced price = %+v, %v; want catalogue", row, err)
	}
	var invalid *RateValidationError
	if _, err := price("gpt-5-pro", "guess"); !errors.As(err, &invalid) || invalid.Code != "rate_source_unknown" {
		t.Errorf("an unknown source = %v, want rate_source_unknown", err)
	}
}

// Same-day writes upsert one row; an admin's correction must take the row's
// provenance with it, or the next sync would overwrite what they just typed.
func TestAnAdminCorrectingASyncedPriceMakesItTheirs(t *testing.T) {
	e := setupRateStore(t)
	ws, _ := e.seedWorkspace(context.Background(), t)
	ctx := laneWriterCtx(ws)
	store := e.storeFor(ws)
	in := SetModelRateInput{
		Provider: "openai", ModelID: "gpt-5-nano", InputUsd: "0.05", OutputUsd: "0.4",
		CacheReadUsd: "0", CacheWriteUsd: "0", Source: RateSourceCatalogue,
	}
	if _, err := store.SetModelRate(ctx, in); err != nil {
		t.Fatal(err)
	}
	in.InputUsd, in.Source = "0.06", ""
	if _, err := store.SetModelRate(ctx, in); err != nil {
		t.Fatal(err)
	}
	rows, err := store.ListEffectiveModelRates(ctx)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, r := range rows {
		if r.ModelID == "gpt-5-nano" {
			found = true
			if r.Source != RateSourceManual {
				t.Errorf("corrected row source = %q, want manual", r.Source)
			}
		}
	}
	if !found {
		t.Fatal("no gpt-5-nano price in force")
	}
}

// The sync plans from a snapshot; an admin may type a price between that read
// and the sync's write. The write, under the model's lock, must still yield.
func TestASyncWriteYieldsToAPriceTypedSinceItsRead(t *testing.T) {
	e := setupRateStore(t)
	ws, _ := e.seedWorkspace(context.Background(), t)
	ctx := laneWriterCtx(ws)
	store := e.storeFor(ws)
	in := SetModelRateInput{
		Provider: "openai", ModelID: "gpt-5-nano", InputUsd: "0.07", OutputUsd: "0.4",
		CacheReadUsd: "0", CacheWriteUsd: "0",
	}
	if _, err := store.SetModelRate(ctx, in); err != nil {
		t.Fatal(err)
	}
	in.InputUsd, in.Source = "0.05", RateSourceCatalogue
	if _, err := store.SetModelRate(ctx, in); !errors.Is(err, errHandSetSinceRead) {
		t.Fatalf("a catalogue write over today's hand-set price = %v, want errHandSetSinceRead", err)
	}
	rows, err := store.ListEffectiveModelRates(ctx)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, r := range rows {
		if r.ModelID == "gpt-5-nano" {
			found = true
			if r.InputUsd != "0.07" || r.Source != RateSourceManual {
				t.Errorf("hand-set price = %+v, want 0.07 kept as manual", r)
			}
		}
	}
	if !found {
		t.Fatal("no gpt-5-nano price in force")
	}
}

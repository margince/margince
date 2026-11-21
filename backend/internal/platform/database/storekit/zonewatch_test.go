// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package storekit

import (
	"context"
	"slices"
	"testing"
)

const zoneNoDatabaseHolds = "Mars/Olympus_Mons"

func TestEffectiveZoneKeepsANameGoLoads(t *testing.T) {
	ctx, watch := WatchZones(context.Background())
	if got := EffectiveZone(ctx, "Europe/Berlin"); got != "Europe/Berlin" {
		t.Errorf("a loadable zone reads as %q, want the stored name", got)
	}
	if names := watch.Unloaded(); len(names) != 0 {
		t.Errorf("a loadable zone was noted as unloaded: %v", names)
	}
}

func TestEffectiveZoneAnswersUTCForANameGoCannotLoad(t *testing.T) {
	ctx, watch := WatchZones(context.Background())
	for range 3 {
		if got := EffectiveZone(ctx, zoneNoDatabaseHolds); got != FallbackZone {
			t.Fatalf("an unloadable zone reads as %q, want %q", got, FallbackZone)
		}
	}
	if names := watch.Unloaded(); !slices.Equal(names, []string{zoneNoDatabaseHolds}) {
		t.Errorf("three reads of one unloadable zone noted %v, want the name once", names)
	}
}

func TestEffectiveZoneKeepsTheStoredNameOutsideAWatch(t *testing.T) {
	if got := EffectiveZone(context.Background(), zoneNoDatabaseHolds); got != zoneNoDatabaseHolds {
		t.Errorf("an unwatched read answers %q, want the stored name, which its caller then refuses", got)
	}
	if ZoneLoads(zoneNoDatabaseHolds) || !ZoneLoads("Europe/Berlin") {
		t.Error("ZoneLoads misreads a name Go lacks or one it holds")
	}
}

// A nil transaction proves a run whose zones all loaded reaches no statement.
func TestRecordTxWritesNothingWhenEveryZoneLoaded(t *testing.T) {
	ctx, watch := WatchZones(context.Background())
	EffectiveZone(ctx, "UTC")
	if err := watch.RecordTx(ctx, nil); err != nil {
		t.Errorf("recording a clean run failed: %v", err)
	}
}

// With no actor bound, LogSystem refuses before any statement, which shows the
// fault is on its way to system_log.
func TestRecordTxWritesTheFaultOfAnUnloadedZone(t *testing.T) {
	ctx, watch := WatchZones(context.Background())
	EffectiveZone(ctx, zoneNoDatabaseHolds)
	if err := watch.RecordTx(ctx, nil); err == nil {
		t.Error("a run that fell back to UTC recorded no fault")
	}
}

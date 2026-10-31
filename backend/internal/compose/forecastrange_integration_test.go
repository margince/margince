// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// The forecast's range reaches every surface from the one assembler. The HTTP
// door and the tool state one range, and a snapshot freezes it. A share draws
// it again over the rows its reader may see.

import (
	"context"
	"testing"

	"github.com/margince/margince/backend/internal/modules/forecasting"
	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// One deal worth 1,000,000 at the typed 20% wins in two of ten outcomes. So
// the low end is nothing and the high end is the whole deal.
func TestTheHTTPDoorAndTheToolStateOneRange(t *testing.T) {
	f := newStageCohortFixture(t)
	today := clocktest.Now(t)
	f.openDeal(t, f.stage(0), today)

	overHTTP, overTool := f.forecastOverHTTP(t, today), f.forecastOverTool(t, today)
	if overHTTP.RangeLowMinor == nil || overHTTP.RangeHighMinor == nil ||
		*overHTTP.RangeLowMinor != 0 || *overHTTP.RangeHighMinor != 1_000_000 {
		t.Fatalf("GET /forecast range %v..%v, want 0..1,000,000", overHTTP.RangeLowMinor, overHTTP.RangeHighMinor)
	}
	if overTool.RangeLowMinor == nil || overTool.RangeHighMinor == nil ||
		*overTool.RangeLowMinor != *overHTTP.RangeLowMinor || *overTool.RangeHighMinor != *overHTTP.RangeHighMinor {
		t.Fatalf("the tool's range %v..%v differs from the HTTP door's %d..%d",
			overTool.RangeLowMinor, overTool.RangeHighMinor, *overHTTP.RangeLowMinor, *overHTTP.RangeHighMinor)
	}
}

// Two deals at 20%, worth 100,000 and 250,000. Both together: nothing in 64%
// of outcomes, 100,000 in 16%, 250,000 in 16%, so the high end is 250,000.
func TestASnapshotFreezesItsRange(t *testing.T) {
	e := setupForecast(t)
	id := e.freezeTwoTeams(t, 100_000, 250_000)

	var low, high int64
	var runs int
	var quantiles, version string
	if err := e.owner.QueryRow(context.Background(),
		`SELECT range_low_minor, range_high_minor, range_runs, range_quantiles, range_version
		   FROM forecast_snapshot WHERE id = $1`, id).Scan(&low, &high, &runs, &quantiles, &version); err != nil {
		t.Fatalf("reading the frozen range: %v", err)
	}
	if low != 0 || high != 250_000 || runs != 4 {
		t.Fatalf("frozen range %d..%d over %d outcomes, want 0..250,000 over the 4 enumerated", low, high, runs)
	}
	if quantiles != forecasting.RangeQuantiles || version != forecasting.RangeVersion {
		t.Fatalf("frozen convention %q and rules %q, want %q and %q", quantiles, version, forecasting.RangeQuantiles, forecasting.RangeVersion)
	}
}

// A team-scoped reader sees only their own 100,000 deal. At 20% it is lost in
// eight of ten outcomes, so their high end is 100,000, not the stored 250,000.
func TestARestrictedShareDrawsItsRangeOverTheRowsItsReaderSees(t *testing.T) {
	e := setupForecast(t)
	id := e.freezeTwoTeams(t, 100_000, 250_000)

	whole := e.readShared(snapshotWriterCtx(e.WS), t, id)
	if whole.Readings.Range == nil || whole.Readings.Range.LowMinor != 0 || whole.Readings.Range.HighMinor != 250_000 {
		t.Fatalf("a reader who sees every frozen row draws %+v, want the frozen 0..250,000", whole.Readings.Range)
	}
	narrow := e.readShared(e.forecastReader(
		e.dealReadCtx(e.Rep1, []ids.UUID{e.Team1}, principal.RowScopeTeam)), t, id)
	if narrow.Readings.Range == nil || narrow.Readings.Range.LowMinor != 0 || narrow.Readings.Range.HighMinor != 100_000 {
		t.Fatalf("a team-scoped reader draws %+v, want 0..100,000 over their own deal alone", narrow.Readings.Range)
	}
}

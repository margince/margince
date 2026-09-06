// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The driver ordering, and the two int64 edges that would invert it silently.

import (
	"math"
	"slices"
	"testing"

	"github.com/margince/margince/backend/internal/compose/weekly"
	"github.com/margince/margince/backend/internal/modules/forecasting"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// A slip of 90k matters as much as a win of 90k, so the order is by magnitude
// and not by signed size — which would bury every loss below every gain.
func TestDriversOrderByHowFarTheyMovedNotWhichWay(t *testing.T) {
	t.Parallel()
	moves := []int64{10_00, -90_00, 50_00}
	slices.SortFunc(moves, biggerMovementFirst)

	want := []int64{-90_00, 50_00, 10_00}
	if !slices.Equal(moves, want) {
		t.Errorf("ordered %v, want %v — sorting by signed size buries every loss "+
			"below every gain", moves, want)
	}
}

// SortFunc takes an int, so a comparator that SUBTRACTS int64 magnitudes has to
// truncate. On a platform where int is 32 bits that truncation drops the high
// word, and a difference of exactly 2^32 compares as zero — the two movements
// then sort as equal and the larger one loses its place. Comparing rather than
// subtracting is what makes the result independent of int's width.
func TestALargeMovementSortsAboveASmallOneWhateverIntIsWide(t *testing.T) {
	t.Parallel()
	// Magnitudes 2^32 apart: the difference is zero in the low 32 bits.
	moves := []int64{1, 1 + (1 << 32)}
	slices.SortFunc(moves, biggerMovementFirst)

	if moves[0] != 1+(1<<32) {
		t.Errorf("the larger movement sorted second — a comparator that subtracts and " +
			"truncates reads a difference of 2^32 as no difference at all")
	}
}

// math.MinInt64 has no positive counterpart: negating it returns itself. An
// unclamped magnitude would be NEGATIVE and sort the largest movement last.
func TestTheMostNegativeMovementIsTreatedAsTheLargest(t *testing.T) {
	t.Parallel()
	if got := magnitude(math.MinInt64); got < 0 {
		t.Fatalf("the magnitude of the most negative movement is %d, a NEGATIVE "+
			"magnitude — it would sort as the smallest movement when it is the largest",
			got)
	}
	moves := []int64{5_00, math.MinInt64}
	slices.SortFunc(moves, biggerMovementFirst)
	if moves[0] != math.MinInt64 {
		t.Error("the most negative movement did not sort first, so a negated magnitude " +
			"inverted the order")
	}
}

// A bucket total is netted before the seam sees it, so folding bars from
// movement.Buckets loses the losing half of a mixed cause.
//
// The engine sums every deal into one figure per cause. Three of the twelve
// buckets pick their bar by SIGN, so a bucket holding a +100 reprice and a -50
// reprice arrives as a single +50 — and a fold reading that total would draw
// one "advanced" bar of 50 where the week actually held an advance of 100 and a
// slip of 50. The slipped bar, and every driver behind it, would be gone.
func TestFoldingMixedRepricesKeepsBothDirections(t *testing.T) {
	t.Parallel()
	movement := forecasting.Movement{
		// What the engine reports: ONE netted bucket.
		Buckets: []forecasting.Bucket{
			{Name: forecasting.BucketAmount, AmountMinor: 50_00, DealCount: 2},
		},
		// And the per-deal deltas behind it, which still carry their signs.
		Deals: []forecasting.DealDelta{
			{DealID: ids.NewV7().String(), Bucket: forecasting.BucketAmount, AmountMinor: 100_00},
			{DealID: ids.NewV7().String(), Bucket: forecasting.BucketAmount, AmountMinor: -50_00},
		},
	}

	bars, err := barsFrom("quarter", movement)
	if err != nil {
		t.Fatalf("folding the movement: %v", err)
	}

	byBar := map[string]int64{}
	for _, bar := range bars {
		byBar[bar.Bar] = bar.DeltaMinor
	}
	if byBar[weekly.BarAdvanced] != 100_00 {
		t.Errorf("the advanced bar is %d, want 10000 — folding a netted bucket total "+
			"reports one direction and loses the other", byBar[weekly.BarAdvanced])
	}
	if byBar[weekly.BarSlipped] != -50_00 {
		t.Errorf("the slipped bar is %d, want -5000 — a week that lost money on a deal "+
			"must not read as a week that only gained", byBar[weekly.BarSlipped])
	}
}

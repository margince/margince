// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package weekly

// Folding twelve buckets into six bars is where money goes missing, so the
// test derives its corpus from the ENGINE and not from the mapping's own keys.
// A map checked against itself agrees with any mistake.
//
// Six is the count of MOVES. The bridge's opening and closing landings are
// anchors read off the frozen snapshots, not folded from a bucket, which is
// what TestTheBridgeNamesOnlyMovesAndNotItsTwoAnchors holds.

import (
	"slices"
	"testing"

	"github.com/margince/margince/backend/internal/modules/forecasting"
)

// The census. A bucket the engine gains and the bridge never names would drop
// its money out of a waterfall that still draws and still sums to something.
func TestEveryForecastBucketLandsInExactlyOneBridgeBar(t *testing.T) {
	t.Parallel()
	bars := BridgeBars()
	for _, bucket := range forecasting.MovementBuckets() {
		bar, err := BarFor(bucket, 1_000)
		if err != nil {
			t.Errorf("the engine reports bucket %q and the bridge has no bar for it: %v — "+
				"its money would vanish from a waterfall that still draws", bucket, err)
			continue
		}
		if !slices.Contains(bars, bar) {
			t.Errorf("bucket %q maps to bar %q, which the bridge never draws", bucket, bar)
		}
	}
}

// The bars are the MOVES only. The opening and closing landings are anchors
// read off the frozen snapshots, so a bar for either would be counted twice —
// once as the anchor the bridge starts from and again as a movement inside it.
func TestTheBridgeNamesOnlyMovesAndNotItsTwoAnchors(t *testing.T) {
	t.Parallel()
	bars := BridgeBars()
	if len(bars) != 6 {
		t.Errorf("the bridge declares %d bars, want the 6 moves — the opening and closing "+
			"landings are anchors, and a bar for either double-counts it", len(bars))
	}
	for _, anchor := range []string{"start", "end", "opening", "closing"} {
		if slices.Contains(bars, anchor) {
			t.Errorf("%q is a bar, but it is an anchor: the bridge would sum it as a "+
				"movement as well as reading it as a landing", anchor)
		}
	}
}

// A reprice down is a slip, not an advance. Same bucket, opposite story: a
// bridge that read the bucket alone would draw a bar of progress out of a week
// somebody lost money in.
func TestARepricingBucketFollowsItsSignRatherThanItsName(t *testing.T) {
	t.Parallel()
	for _, bucket := range []string{
		forecasting.BucketAmount,
		forecasting.BucketCategory,
		forecasting.BucketStageWeight,
	} {
		up, err := BarFor(bucket, 5_000)
		if err != nil {
			t.Fatalf("bucket %q: %v", bucket, err)
		}
		down, err := BarFor(bucket, -5_000)
		if err != nil {
			t.Fatalf("bucket %q: %v", bucket, err)
		}
		if up != BarAdvanced {
			t.Errorf("bucket %q gaining money draws in %q, want %q", bucket, up, BarAdvanced)
		}
		if down != BarSlipped {
			t.Errorf("bucket %q LOSING money draws in %q, want %q — a bridge reading the "+
				"bucket without its sign turns a bad week green", bucket, down, BarSlipped)
		}
	}
}

// The decided outcomes never move, whatever their sign. A won deal is won even
// where the delta is negative because the period lost its open value.
func TestADecidedOutcomeKeepsItsBarWhicheverWayTheMoneyWent(t *testing.T) {
	t.Parallel()
	for bucket, want := range map[string]string{
		forecasting.BucketWon:  BarWon,
		forecasting.BucketLost: BarLost,
	} {
		for _, delta := range []int64{5_000, -5_000} {
			got, err := BarFor(bucket, delta)
			if err != nil {
				t.Fatalf("bucket %q: %v", bucket, err)
			}
			if got != want {
				t.Errorf("bucket %q at delta %d draws in %q, want %q",
					bucket, delta, got, want)
			}
		}
	}
}

// The machinery buckets stay out of the bars a person is judged by. An FX move
// credited as an advance tells a rep they sold something a rate did.
func TestTheMachineryBucketsAreTheirOwnBarAndNotProgress(t *testing.T) {
	t.Parallel()
	for _, bucket := range []string{
		forecasting.BucketFx,
		forecasting.BucketDefinition,
		forecasting.BucketModel,
		forecasting.BucketArchived,
	} {
		bar, err := BarFor(bucket, 5_000)
		if err != nil {
			t.Fatalf("bucket %q: %v", bucket, err)
		}
		if bar != BarOther {
			t.Errorf("bucket %q draws in %q, want %q — nobody DID this, and crediting it "+
				"as progress tells a rep they sold what a rate move did", bucket, bar, BarOther)
		}
	}
}

// An unknown bucket is refused rather than silently dropped or defaulted into
// "other", which would hide the very drift the census exists to catch.
func TestAnUnknownBucketIsRefusedRatherThanFoldedIntoOther(t *testing.T) {
	t.Parallel()
	if _, err := BarFor("invented_bucket", 1_000); err == nil {
		t.Fatal("an unknown bucket answered a bar instead of refusing — defaulting one " +
			"into 'other' hides a bucket the engine gained and the bridge never learned")
	}
}

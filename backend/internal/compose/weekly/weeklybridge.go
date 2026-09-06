// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package weekly

// The movement bridge: how a week got from its opening landing to its closing
// one, in the six moves a person tells the story in.
//
// SIX, not the eight the waterfall draws. The opening and closing landings are
// the bridge's two ANCHORS — they are read off the frozen snapshots either side
// of the week, not folded from any bucket — so they carry no bar here. A reader
// counting the drawn columns sees eight; this file names only what MOVED.
//
// The forecast engine reports twelve buckets, which is the right grain for
// diagnosing a quarter and too fine for a weekly retrospective — nobody opens a
// Monday review to learn that 3% of the change was an FX revaluation. So the
// twelve fold into these six.
//
// Folding is where money goes missing. A bar list that quietly drops a bucket
// still sums to something and still draws, so the mapping is total by
// construction: every bucket names its bar here, and a bucket added to the
// engine with no bar named fails the gate rather than disappearing from a
// waterfall that looks fine.

import (
	"fmt"

	"github.com/margince/margince/backend/internal/modules/forecasting"
)

// The bars, in the order the bridge draws them.
const (
	// BarCreated is pipeline that did not exist at the start of the week.
	BarCreated = "created"
	// BarAdvanced is a deal worth more to the period than it was: repriced up,
	// upgraded, or moved to a stage that weights it higher.
	BarAdvanced = "advanced"
	// BarSlipped is the mirror — pushed out of the period, downgraded, or
	// repriced down. Named for what a rep would say happened.
	BarSlipped = "slipped"
	// BarWon and BarLost are the two decided outcomes.
	BarWon  = "won"
	BarLost = "lost"
	// BarOther is the machinery: FX revaluation, a definition change, a model
	// change. Real money, but not something anybody DID this week — kept as its
	// own bar rather than spread across the others, which would credit a rep
	// with a rate move.
	BarOther = "other"
)

// barOrder is the drawing order of the MOVES.
//
// Left to right it is the story a review is read as: what we made, what moved,
// what we closed, and what happened to us. The opening and closing anchors sit
// either side of these and are not members: they are landings, not movements,
// and a caller summing this list must arrive at the closing anchor from the
// opening one rather than finding both inside it.
var barOrder = []string{
	BarCreated, BarAdvanced, BarSlipped, BarWon, BarLost, BarOther,
}

// BridgeBars is the set of bars, in drawing order.
//
// Held by: TestEveryForecastBucketLandsInExactlyOneBridgeBar
// (backend/internal/compose/weekly/weeklybridge_test.go), which derives the
// buckets from forecasting.MovementBuckets and requires each to name a bar this
// returns — so a bar added here and never mapped to, or a bucket mapped to a
// bar this omits, fails rather than dropping money from a waterfall.
func BridgeBars() []string { return append([]string{}, barOrder...) }

// bucketBar maps each forecast bucket onto the bar it is told as.
//
// Held by TestEveryForecastBucketLandsInExactlyOneBridgeBar, which derives the
// left side from forecasting.MovementBuckets rather than from this map's own
// keys — a map checked against itself would agree with any mistake.
var bucketBar = map[string]string{
	forecasting.BucketNew: BarCreated,
	// Pulled INTO the period is the period gaining a deal, which reads as
	// progress; pushed out is the classic slip.
	forecasting.BucketPulledIn:  BarAdvanced,
	forecasting.BucketPushedOut: BarSlipped,
	// A reprice or a category or stage move can go either way, so the SIGN
	// decides the bar rather than the bucket. signedBar below does that; the
	// entry here is the direction a positive delta takes.
	forecasting.BucketAmount:      BarAdvanced,
	forecasting.BucketCategory:    BarAdvanced,
	forecasting.BucketStageWeight: BarAdvanced,
	forecasting.BucketWon:         BarWon,
	forecasting.BucketLost:        BarLost,
	// An archived or reopened deal is neither won nor lost and nobody sold it.
	forecasting.BucketArchived:   BarOther,
	forecasting.BucketFx:         BarOther,
	forecasting.BucketDefinition: BarOther,
	forecasting.BucketModel:      BarOther,
}

// bucketsWhoseSignPicksTheBar are the three whose direction is not fixed.
//
// A reprice down is a slip and a reprice up is an advance, and they are the
// same bucket. Told by sign rather than by bucket, because a bridge that put
// every reprice under "advanced" would draw a bar of progress out of a week
// somebody lost money in.
var bucketsWhoseSignPicksTheBar = map[string]bool{
	forecasting.BucketAmount:      true,
	forecasting.BucketCategory:    true,
	forecasting.BucketStageWeight: true,
}

// BarFor answers which bar a bucket's delta is drawn in.
//
// The delta's SIGN is part of the question for the three repricing buckets, so
// it is taken rather than inferred: a caller holding only the bucket name
// cannot answer this, and letting one guess is how a losing week draws green.
func BarFor(bucket string, deltaMinor int64) (string, error) {
	bar, known := bucketBar[bucket]
	if !known {
		return "", fmt.Errorf(
			"weekly: %q is not a forecast movement bucket, so the bridge has no bar for "+
				"it and its money would vanish from a waterfall that still draws", bucket)
	}
	if bucketsWhoseSignPicksTheBar[bucket] && deltaMinor < 0 {
		return BarSlipped, nil
	}
	return bar, nil
}

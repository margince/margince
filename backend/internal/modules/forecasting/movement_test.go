// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package forecasting

import (
	"math/rand"
	"testing"
	"time"
)

// open builds a contribution that is in the open pipeline at a base amount.
func open(id string, base int64) Contribution {
	return Contribution{
		DealID: id, AmountMinor: &base, Currency: "EUR", BaseMinor: &base,
		Category: CategoryCommit, StageProbability: 50, WeightedMinor: base / 2,
		InOpen: true, InEvidence: true, InBestCase: true,
	}
}

func side(version string, rows ...Contribution) snapshotSide {
	return snapshotSide{DefinitionVersion: version, Contributions: rows}
}

// bucketOf answers which bucket a deal landed in, and fails if it landed in
// none or in several — "exactly once" is the rule the identity rests on.
func bucketOf(t *testing.T, m Movement, dealID string) DealDelta {
	t.Helper()
	var found []DealDelta
	for _, d := range m.Deals {
		if d.DealID == dealID {
			found = append(found, d)
		}
	}
	if len(found) != 1 {
		t.Fatalf("deal %s appears in %d deltas, want exactly 1 — a deal counted twice "+
			"doubles its money and breaks the waterfall's identity", dealID, len(found))
	}
	return found[0]
}

// reconciles is the property every case below also asserts: opening plus every
// bucket equals closing, exactly. Both sides are sums of stored integers, so
// this is an identity and not an approximation.
func reconciles(t *testing.T, m Movement) {
	t.Helper()
	total := m.OpeningMinor
	for _, b := range m.Buckets {
		total += b.AmountMinor
	}
	if total != m.ClosingMinor {
		t.Errorf("opening %d plus the buckets is %d, and closing is %d — a waterfall whose "+
			"bars do not reach the closing bar is a picture that has to be explained away",
			m.OpeningMinor, total, m.ClosingMinor)
	}
}

func TestEachBucketInIsolation(t *testing.T) {
	t.Parallel()
	const v = DefinitionVersion

	for _, tc := range []struct {
		name       string
		from, to   snapshotSide
		dealID     string
		wantBucket string
		// Which reading the change is asserted on. Zero value means open.
		reading Reading
	}{
		{
			name: "a deal that was not there before",
			from: side(v), to: side(v, open("d1", 10_000)),
			dealID: "d1", wantBucket: BucketNew,
		},
		{
			name: "a repricing",
			from: side(v, open("d1", 10_000)), to: side(v, open("d1", 15_000)),
			dealID: "d1", wantBucket: BucketAmount,
		},
		{
			// Category is asserted on the EVIDENCE reading, where it moves
			// money: a deal dropping from commit to best_case leaves the
			// evidence total. On the open reading the same change moves
			// nothing, and a waterfall bar of zero is noise rather than news.
			name: "a category change, on the reading it moves",
			from: side(v, open("d1", 10_000)),
			to: side(v, withoutEvidence(
				withCategory(open("d1", 10_000), CategoryBestCase))),
			dealID: "d1", wantBucket: BucketPushedOut, reading: ReadingEvidence,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			reading := tc.reading
			if reading == "" {
				reading = ReadingOpen
			}
			m := Classify(reading, tc.from, tc.to)
			if got := bucketOf(t, m, tc.dealID).Bucket; got != tc.wantBucket {
				t.Errorf("deal landed in %q, want %q", got, tc.wantBucket)
			}
			reconciles(t, m)
		})
	}
}

// A stage's probability moving is a WEIGHTED-reading event. It changes no open
// pipeline at all, which is why the bucket has to be tested on the reading it
// actually moves.
func TestAStageWeightChangeMovesTheWeightedReading(t *testing.T) {
	t.Parallel()
	was := open("d1", 10_000)
	now := open("d1", 10_000)
	now.StageProbability = 80
	now.WeightedMinor = 8_000

	m := Classify(ReadingWeighted, side(DefinitionVersion, was), side(DefinitionVersion, now))
	if got := bucketOf(t, m, "d1").Bucket; got != BucketStageWeight {
		t.Errorf("a probability change landed in %q, want %q", got, BucketStageWeight)
	}
	reconciles(t, m)
}

// The escape cases. Each one silently breaks the identity if it is not handled,
// which is why they are planted rather than left to a generator to stumble on.

func TestADealArchivedBetweenSnapshotsLeavesThroughABucket(t *testing.T) {
	t.Parallel()
	// Absent from the closing snapshot entirely: archived, or moved out of the
	// caller's scope. Without a bucket its money vanishes and the waterfall
	// stops reconciling with no row to explain where it went.
	m := Classify(ReadingOpen, side(DefinitionVersion, open("d1", 10_000)), side(DefinitionVersion))
	delta := bucketOf(t, m, "d1")
	if delta.Bucket != BucketArchived {
		t.Errorf("a vanished deal landed in %q, want %q", delta.Bucket, BucketArchived)
	}
	if delta.AmountMinor != -10_000 {
		t.Errorf("it took %d out, want -10000 — its whole prior contribution", delta.AmountMinor)
	}
	reconciles(t, m)
}

func TestADealThatBecameUnpricedLeavesThroughAmount(t *testing.T) {
	t.Parallel()
	// Still eligible, still in the period, contributing nothing. Its full prior
	// value has to leave through a bucket a reader can see, not through an
	// exclusion nobody drew.
	now := open("d1", 10_000)
	now.AmountMinor = nil
	now.BaseMinor = nil
	now.WeightedMinor = 0
	now.ExclusionReason = ExcludedUnpriced

	m := Classify(ReadingOpen, side(DefinitionVersion, open("d1", 10_000)), side(DefinitionVersion, now))
	delta := bucketOf(t, m, "d1")
	if delta.Bucket != BucketAmount {
		t.Errorf("a deal that lost its price landed in %q, want %q", delta.Bucket, BucketAmount)
	}
	if delta.AmountMinor != -10_000 {
		t.Errorf("it took %d out, want -10000", delta.AmountMinor)
	}
	reconciles(t, m)
}

func TestADealCreatedAndWonBetweenSnapshotsAppearsOnce(t *testing.T) {
	t.Parallel()
	// Both "new" and "won" are true of it. Counted in both, its money doubles.
	won := open("d1", 10_000)
	won.InOpen, won.InEvidence, won.InBestCase = false, false, false
	won.InWon = true

	m := Classify(ReadingOpen, side(DefinitionVersion), side(DefinitionVersion, won))
	// bucketOf fails the test if it appears twice, which is the assertion.
	if len(m.Deals) > 1 {
		t.Errorf("a deal created and won between two snapshots produced %d deltas", len(m.Deals))
	}
	reconciles(t, m)
}

func TestADifferentDefinitionVersionTakesTheWholeDifference(t *testing.T) {
	t.Parallel()
	// A rules change moves numbers without the business moving. Spread across
	// the business buckets it would be reported as sales activity, which is the
	// one thing a manager reading a waterfall must never be told.
	m := Classify(ReadingOpen,
		side("forecast_v1", open("d1", 10_000)),
		side("forecast_v2", open("d1", 15_000)))

	if len(m.Buckets) != 1 || m.Buckets[0].Name != BucketDefinition {
		t.Fatalf("a definition change produced buckets %v, want only %q", m.Buckets, BucketDefinition)
	}
	if m.Buckets[0].AmountMinor != 5_000 {
		t.Errorf("the definition bucket took %d, want the whole 5000 difference", m.Buckets[0].AmountMinor)
	}
	reconciles(t, m)
}

// The property, over random populations. Every case above is one shape; this is
// the claim that holds for shapes nobody wrote down.
func TestTheBucketsAlwaysReachTheClosingTotal(t *testing.T) {
	t.Parallel()
	rng := rand.New(rand.NewSource(20260903))

	for run := range 300 {
		var before, after []Contribution
		for i := range rng.Intn(8) + 1 {
			id := string(rune('a' + i))
			base := int64(rng.Intn(100_000))
			was := open(id, base)

			switch rng.Intn(6) {
			case 0: // unchanged
				before, after = append(before, was), append(after, was)
			case 1: // repriced
				now := open(id, base+int64(rng.Intn(50_000)))
				before, after = append(before, was), append(after, now)
			case 2: // vanished
				before = append(before, was)
			case 3: // arrived
				after = append(after, was)
			case 4: // won
				now := was
				now.InOpen, now.InEvidence, now.InBestCase = false, false, false
				now.InWon = true
				before, after = append(before, was), append(after, now)
			case 5: // became unpriced
				now := was
				now.AmountMinor, now.BaseMinor, now.WeightedMinor = nil, nil, 0
				now.ExclusionReason = ExcludedUnpriced
				before, after = append(before, was), append(after, now)
			}
		}
		for _, reading := range []Reading{ReadingOpen, ReadingWeighted, ReadingEvidence} {
			m := Classify(reading, side(DefinitionVersion, before...), side(DefinitionVersion, after...))
			total := m.OpeningMinor
			for _, b := range m.Buckets {
				total += b.AmountMinor
			}
			if total != m.ClosingMinor {
				t.Fatalf("run %d, reading %s: opening %d plus buckets is %d, closing is %d",
					run, reading, m.OpeningMinor, total, m.ClosingMinor)
			}
		}
	}
}

func withCategory(c Contribution, category string) Contribution {
	c.Category = category
	return c
}

// withoutEvidence is what a drop from commit to best_case does: the deal stays
// open pipeline and leaves the reading that claims confirmed support.
func withoutEvidence(c Contribution) Contribution {
	c.InEvidence = false
	return c
}

var _ = time.Now

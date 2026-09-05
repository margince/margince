// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package forecasting

import (
	"sort"
	"time"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// The buckets a change can land in, in the order a waterfall draws them.
//
// Fixed order because a waterfall is read left to right as a story: what
// arrived, what moved in and out of the period, what was repriced, and only
// then what the machinery did. Two installations drawing the same movement in
// different orders would be telling two different stories about one quarter.
const (
	BucketNew         = "new"
	BucketPulledIn    = "pulled_in"
	BucketPushedOut   = "pushed_out"
	BucketAmount      = "amount"
	BucketCategory    = "category"
	BucketStageWeight = "stage_weight"
	BucketWon         = "won"
	BucketLost        = "lost"
	BucketArchived    = "reopened_or_archived"
	BucketFx          = "fx"
	BucketDefinition  = "definition"
	BucketModel       = "model"
)

// bucketOrder is the drawing order, and the ONE place it is stated.
var bucketOrder = []string{
	BucketNew, BucketPulledIn, BucketPushedOut, BucketAmount, BucketCategory,
	BucketStageWeight, BucketWon, BucketLost, BucketArchived, BucketFx,
	BucketDefinition, BucketModel,
}

// Movement is the difference between two snapshots, classified.
//
// Opening plus every bucket equals Closing, exactly. Both sides are sums of
// stored integers, so this is an identity a test can hold rather than an
// approximation — and it is the whole point: a waterfall whose bars do not
// reach the closing bar is a picture that has to be explained away.
type Movement struct {
	OpeningMinor int64
	ClosingMinor int64
	Buckets      []Bucket
	Deals        []DealDelta
}

// Bucket is one named cause and what it moved.
type Bucket struct {
	Name        string
	AmountMinor int64
	DealCount   int
}

// DealDelta is one deal's contribution to the difference, in exactly one
// bucket.
//
// Exactly one, and that is a rule rather than an observation: a deal that
// changed price AND slipped out of the period has moved for one reason as far
// as a reader is concerned — it left. Counting it twice would double its money
// and break the identity above.
type DealDelta struct {
	DealID      string
	Bucket      string
	AmountMinor int64
	// What the reader needs to see the change: the two sides of it.
	FromMinor *int64
	ToMinor   *int64
	// Who moved it and what authorised them, carried from the closing
	// snapshot's stored reference rather than reconstructed from timestamps.
	AuditID    *ids.UUID
	ApprovalID *ids.UUID
}

// snapshotSide is one end of the comparison: which reading is being explained,
// and the rows behind it.
type snapshotSide struct {
	DefinitionVersion string
	Contributions     []Contribution
	// The window this side was frozen over. Carried so a comparison can refuse
	// two snapshots that are not about the same period: differencing a week
	// against a quarter reports the change of WINDOW as deal movement, and
	// every line of the waterfall would be a number nobody can act on.
	PeriodStart time.Time
	PeriodEnd   time.Time
}

// Reading names which of the four money answers a movement explains. A
// waterfall is drawn for ONE of them, and mixing two would add figures that do
// not belong in the same total.
type Reading string

// The four money answers a movement can explain.
const (
	ReadingOpen     Reading = "open"
	ReadingWeighted Reading = "weighted"
	ReadingEvidence Reading = "evidence"
	ReadingBestCase Reading = "best_case"
)

// Classify explains the difference between two snapshots of one reading.
func Classify(reading Reading, from, to snapshotSide) Movement {
	before := indexByDeal(from.Contributions)
	after := indexByDeal(to.Contributions)

	// A definition change moves numbers without the business moving, so the
	// WHOLE difference belongs to that bucket. Spreading it across the business
	// buckets would report a rules change as sales activity — which is the one
	// thing a manager reading a waterfall must never be told.
	definitionChanged := from.DefinitionVersion != to.DefinitionVersion

	out := Movement{
		OpeningMinor: totalOf(reading, from.Contributions),
		ClosingMinor: totalOf(reading, to.Contributions),
	}
	if definitionChanged {
		out.Buckets = []Bucket{{
			Name:        BucketDefinition,
			AmountMinor: out.ClosingMinor - out.OpeningMinor,
			DealCount:   len(after),
		}}
		return out
	}

	byBucket := map[string]*Bucket{}
	add := func(name string, delta int64) {
		if byBucket[name] == nil {
			byBucket[name] = &Bucket{Name: name}
		}
		byBucket[name].AmountMinor += delta
		byBucket[name].DealCount++
	}

	for dealID, now := range after {
		was, existed := before[dealID]
		delta := contributionOf(reading, now) - contributionOf(reading, was)
		if !existed {
			delta = contributionOf(reading, now)
		}
		if delta == 0 && existed {
			continue
		}
		bucket := classifyOne(reading, was, existed, now)
		add(bucket, delta)
		out.Deals = append(out.Deals, dealDelta(dealID, bucket, delta, reading, was, existed, now))
	}
	// A deal in the opening and absent from the closing left the population
	// entirely — archived, or moved out of the caller's scope. Its whole prior
	// contribution has to leave through a bucket, or the identity breaks with
	// no row to explain where the money went.
	for dealID, was := range before {
		if _, still := after[dealID]; still {
			continue
		}
		delta := -contributionOf(reading, was)
		if delta == 0 {
			continue
		}
		add(BucketArchived, delta)
		out.Deals = append(out.Deals, DealDelta{
			DealID: dealID, Bucket: BucketArchived, AmountMinor: delta,
			FromMinor: ptr(contributionOf(reading, was)), ToMinor: ptr(int64(0)),
		})
	}

	for _, name := range bucketOrder {
		if b := byBucket[name]; b != nil {
			out.Buckets = append(out.Buckets, *b)
		}
	}
	sort.Slice(out.Deals, func(i, j int) bool { return out.Deals[i].DealID < out.Deals[j].DealID })
	return out
}

// classifyOne answers the ONE bucket a deal's change belongs to.
//
// The order below is the priority, and it is not arbitrary. A deal that both
// closed and changed price closed — that is what a reader would say happened to
// it. Working down: the outcome, then whether it left, then whether it crossed
// the period, then what about it was repriced, and only then the machinery.
func classifyOne(reading Reading, was Contribution, existed bool, now Contribution) string {
	if outcome := classifyOutcome(reading, was, existed, now); outcome != "" {
		return outcome
	}
	return classifyRepricing(was, now)
}

// classifyOutcome answers the buckets that are about what HAPPENED to a deal —
// it closed, it arrived, it left the reading. Empty when none applies, and the
// repricing half decides instead.
func classifyOutcome(reading Reading, was Contribution, existed bool, now Contribution) string {
	switch {
	case now.InWon && (!existed || !was.InWon):
		return BucketWon
	case !existed:
		return BucketNew
	case now.ExclusionReason == "" && was.ExclusionReason != "":
		// It became countable again: a price arrived, or a rate did.
		return BucketAmount
	case inReading(reading, was) && !inReading(reading, now):
		// It left this reading without closing. Either its date moved out of
		// the period or it stopped qualifying — pushed out, from the reader's
		// side of the screen.
		return BucketPushedOut
	case !inReading(reading, was) && inReading(reading, now):
		return BucketPulledIn
	default:
		return ""
	}
}

// classifyRepricing answers what about a deal changed, for one that stayed put.
// The order is the priority: the deal's own money first, then how it is filed,
// then the machinery.
func classifyRepricing(was, now Contribution) string {
	switch {
	case amountChanged(was, now):
		return BucketAmount
	case was.Category != now.Category:
		return BucketCategory
	case was.StageProbability != now.StageProbability:
		return BucketStageWeight
	case fxChanged(was, now):
		return BucketFx
	default:
		// Nothing the business did explains it, and the definition version is
		// unchanged, so what moved was a model output — a probability the
		// product re-scored. Its own bucket, so a model change cannot
		// masquerade as sales movement.
		return BucketModel
	}
}

// amountChanged answers whether the deal's own money moved, including a deal
// that BECAME unpriced: its full prior value leaves through this bucket rather
// than through an exclusion nobody drew.
func amountChanged(was, now Contribution) bool {
	switch {
	case was.AmountMinor == nil && now.AmountMinor == nil:
		return false
	case was.AmountMinor == nil || now.AmountMinor == nil:
		return true
	default:
		return *was.AmountMinor != *now.AmountMinor || was.Currency != now.Currency
	}
}

// fxChanged answers whether only the conversion moved. Checked after the
// amount, so a deal whose price AND rate both moved reads as a repricing —
// which is what happened to it commercially.
func fxChanged(was, now Contribution) bool {
	if was.BaseMinor == nil || now.BaseMinor == nil {
		return was.BaseMinor != now.BaseMinor
	}
	return *was.BaseMinor != *now.BaseMinor
}

// inReading answers whether a contribution is part of one reading.
func inReading(reading Reading, c Contribution) bool {
	switch reading {
	case ReadingEvidence:
		return c.InEvidence
	case ReadingBestCase:
		return c.InBestCase
	case ReadingWeighted, ReadingOpen:
		return c.InOpen
	default:
		return false
	}
}

// contributionOf is one deal's money in one reading, and zero when it is not in
// it — a deal outside a reading contributes nothing to it, which is different
// from contributing an unknown amount.
func contributionOf(reading Reading, c Contribution) int64 {
	if !inReading(reading, c) {
		return 0
	}
	if reading == ReadingWeighted {
		return c.WeightedMinor
	}
	// An unpriced or unconvertible deal is IN the reading and contributes
	// nothing to its money — which is exactly what makes eligible_count and
	// priced_count worth printing beside the total.
	if c.BaseMinor == nil {
		return 0
	}
	return *c.BaseMinor
}

func totalOf(reading Reading, rows []Contribution) int64 {
	var total int64
	for _, row := range rows {
		total += contributionOf(reading, row)
	}
	return total
}

func indexByDeal(rows []Contribution) map[string]Contribution {
	out := make(map[string]Contribution, len(rows))
	for _, row := range rows {
		out[row.DealID] = row
	}
	return out
}

func dealDelta(
	dealID, bucket string, delta int64, reading Reading,
	was Contribution, existed bool, now Contribution,
) DealDelta {
	out := DealDelta{
		DealID: dealID, Bucket: bucket, AmountMinor: delta,
		ToMinor:    ptr(contributionOf(reading, now)),
		AuditID:    now.AuditID,
		ApprovalID: now.ApprovalID,
	}
	if existed {
		out.FromMinor = ptr(contributionOf(reading, was))
	}
	return out
}

func ptr[T any](v T) *T { return &v }

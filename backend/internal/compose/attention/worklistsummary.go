// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package attention

// The one sentence above the queue, and the scope it is counted over.

import (
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
)

// summarize counts the day in the figures the header states.
//
// OVER `considered`, which is every candidate this read weighed — not the page.
// The sentence carries five figures and `total` was always the day's, so
// counting the four bands over the page put two scopes in one sentence: a rep
// with 118 urgent items behind Show more was told "11 urgent … 165 in all", and
// the four figures did not move as they paged while the total did not move
// either, for the opposite reason.
//
// The older rule this replaces — count what the queue CARRIES, so the line and
// the rows cannot disagree — was written when a twelve-item page was reported
// as a total and the rest was unreachable. The rest is reachable now, and the
// line's own contract says these figures describe the whole assembled day.
//
// Held by: TestTheSummaryCountsTheDayAndNotThePage
// (backend/internal/compose/attention/worklist_test.go).
func summarize(rows []ranked, bar materialBar) crmcontracts.WorklistSummary {
	// Always sent, never omitted: the field is optional in the contract so an
	// older client keeps working, but this server always counts it, and a
	// missing figure would read as "no work in the middle" rather than as "this
	// server does not answer that".
	inPlay := 0
	summary := crmcontracts.WorklistSummary{Total: len(rows), InPlay: &inPlay}
	// The partition, counted in the same walk as the four signals beside it so
	// the two cannot be taken over different populations.
	buckets := crmcontracts.WorklistBuckets{}
	bar.stateOn(&summary)
	for _, row := range rows {
		item := row.item
		// The row's OWN level, not the one a pin may have overwritten. These
		// figures answer what the day holds — `urgent` means somebody is
		// waiting or a promise is breaking — and a rep pinning a piece of
		// hygiene to the top of their morning has made neither true. Counting
		// the sort level here let one reader's ordering preference move a
		// number their manager reads.
		level := item.Level
		if row.pinned {
			level = row.semanticLevel
		}
		// Every level reaches one of the three arms, so no row is missing from
		// the line. Without the default, levels 3 to 5 — material risk, agreed
		// work, blocking decisions — fell between the two and a queue holding
		// only at-risk deals reported three zeros over a page full of rows.
		switch {
		case level <= levelPromise:
			summary.Urgent++
		case level >= levelRoutine:
			summary.LowerPriority++
		default:
			inPlay++
		}
		// Asked of every item whatever its level, so an overdue promise counts
		// here AND above. These are four questions about the day rather than
		// four slices of it, which is what the contract says.
		if item.Overdue != nil && *item.Overdue {
			summary.Due++
		}
		bucketOf(row, level, &buckets)
	}
	summary.Buckets = &buckets
	return summary
}

// bucketOf puts one row in exactly one bucket.
//
// A PRECEDENCE, not four tests. The figures beside it ask four independent
// questions about the day and deliberately double-count — an overdue promise is
// both urgent and due. These four slice the day instead, so the first arm that
// matches takes the row and the four sum to `total`. Written as one switch
// because that is the shape the property needs: no row can fall through, and no
// row can be taken twice.
//
// DESTINATION LEADS, because "is this seller work" outranks "how soon". A
// duplicate pair somebody must judge is not urgent seller work however long it
// has waited, and counting it as urgent would put it in a sentence a rep reads
// as their morning.
//
// Held by TestTheBucketsPartitionTheDay.
func bucketOf(row ranked, level int, buckets *crmcontracts.WorklistBuckets) {
	if destinationOf(row) != destinationToday {
		buckets.Review++
		return
	}
	switch {
	case level <= levelPromise:
		buckets.Urgent++
	case row.item.Overdue != nil && *row.item.Overdue:
		buckets.DueToday++
	default:
		buckets.Planned++
	}
}

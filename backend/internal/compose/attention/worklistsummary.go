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
	bar.stateOn(&summary)
	for _, row := range rows {
		item := row.item
		// Every level reaches one of the three arms, so no row is missing from
		// the line. Without the default, levels 3 to 5 — material risk, agreed
		// work, blocking decisions — fell between the two and a queue holding
		// only at-risk deals reported three zeros over a page full of rows.
		switch {
		case item.Level <= levelPromise:
			summary.Urgent++
		case item.Level >= levelRoutine:
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
	}
	return summary
}

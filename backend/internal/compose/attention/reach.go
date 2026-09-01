// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package attention

import (
	"sort"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
)

// reachOf says, per source, how much of it this read actually looked at.
//
// The queue is a page over a bounded read, and those are two different cuts.
// `considered` is how many candidates the producer handed over and the ranking
// weighed; `shown` is how many survived folding, filtering and the page. A
// reader who sees three rows from a source that considered two hundred has not
// lost the other hundred and ninety-seven — they were folded, filtered, or sit
// below the cut.
//
// `more_available` is the honest half. A lane that came back exactly at its
// bound may have had more behind it, and this read has no way to know how many:
// the bound is a limit on work, not a count of what exists. So the number never
// travels as a total, and a client renders a bounded source as "200+".
//
// Nothing here re-reads a store. Every figure comes from the rows already in
// hand, because a count that costs a second query is a count that can disagree
// with the page it describes.
func reachOf(considered, shown []ranked, bounds map[crmcontracts.WorklistItemSource]bool) []crmcontracts.WorklistReach {
	seen := map[crmcontracts.WorklistItemSource]*crmcontracts.WorklistReach{}
	order := []crmcontracts.WorklistItemSource{}
	at := func(source crmcontracts.WorklistItemSource) *crmcontracts.WorklistReach {
		if row, ok := seen[source]; ok {
			return row
		}
		row := &crmcontracts.WorklistReach{Source: crmcontracts.WorklistReachSource(source), MoreAvailable: bounds[source]}
		seen[source] = row
		order = append(order, source)
		return row
	}
	// A source read successfully and found empty is not the same as a source
	// never read: one says "nothing today", the other says nothing at all. The
	// bounds table names every lane this read asked, so a zero row here is the
	// honest answer rather than an absence the reader has to interpret.
	for source := range bounds {
		at(source)
	}
	for _, row := range considered {
		at(row.item.Source).Considered++
	}
	for _, row := range shown {
		// A folded group stands for its members, and its members' source is
		// what the reader is really being shown some of. Counting the group as
		// one row of source `batch` would report every folded source as shown
		// zero, which reads as "nothing from this source" rather than "folded".
		source := row.item.Source
		if row.item.Batch != nil && len(row.foldedFrom) > 0 {
			for _, member := range row.foldedFrom {
				at(member).Shown++
			}
			continue
		}
		at(source).Shown++
	}
	out := make([]crmcontracts.WorklistReach, 0, len(order))
	for _, source := range order {
		out = append(out, *seen[source])
	}
	// Ordered by source so two reads of one unchanged day produce the same
	// bytes: map order is not an order, and a client diffing the payload would
	// see a change that is not one.
	sort.Slice(out, func(i, j int) bool { return out[i].Source < out[j].Source })
	return out
}

// countsOf says, per CATEGORY, how much work the day held and how much of it
// reached the page.
//
// reach answers the same question per source, and a client cannot turn one into
// the other: the source-to-category map lives here, and summing `reach` in the
// browser would be a second copy of it that drifts the first time a source
// changes lane. So the server states both.
//
// This is the figure the filter pills draw and the completeness line reads. Its
// absence is the defect it exists for: the page is a cut at 25 rows and said
// nothing about what did not fit, so a full first page made a real backlog look
// like zero. A rep narrowing to Decisions found work they had no way to know
// was there.
//
// `considered` is taken BEFORE the category narrowing, which is what lets a
// filtered page still report the categories it is not showing. Counting after
// it would answer "no tasks" on a page filtered to meetings, when the honest
// answer is "tasks, not shown".
func countsOf(
	considered, shown []ranked, bounds map[crmcontracts.WorklistItemSource]bool,
) []crmcontracts.WorklistCount {
	seen := map[crmcontracts.WorklistItemCategory]*crmcontracts.WorklistCount{}
	at := func(category crmcontracts.WorklistItemCategory) *crmcontracts.WorklistCount {
		if row, ok := seen[category]; ok {
			return row
		}
		row := &crmcontracts.WorklistCount{Category: crmcontracts.WorklistCountCategory(category)}
		seen[category] = row
		return row
	}
	// Seeded from the SOURCES first, the way reach is, and not from the rows.
	//
	// A bounded source that contributed no surviving row still truncated its
	// read, and a count built only from rows cannot know that: the scope filter
	// runs before this snapshot, so a lane can come back full of a colleague's
	// work and leave nothing behind. Counting rows alone then reports an exact
	// figure over a category whose read stopped early — which is the one thing
	// these numbers exist to prevent.
	for source, bounded := range bounds {
		if !bounded {
			continue
		}
		at(categoryOfSource(source)).MoreAvailable = true
	}
	for _, row := range considered {
		count := at(row.item.Category)
		count.Considered++
		// A category inherits its sources' honesty. Where any source behind it
		// came back at its bound, the category's own figure is a floor too —
		// otherwise a page would report "12 decisions" over a pile the read
		// never finished counting.
		if bounds[row.item.Source] {
			count.MoreAvailable = true
		}
	}
	for _, row := range shown {
		// A folded group is ONE row standing for many, and the reader is being
		// shown all of them at once. Counting the group as a single shown item
		// would report a category as barely present on a page where it is the
		// dominant thing.
		//
		// Attributed to the BATCH ROW's own category rather than to each
		// member's, which reach has to do: a group only ever folds rows that
		// share a category (batchKeyOf gates on it), so the row's category is
		// its members'.
		if row.item.Batch != nil && row.item.Batch.Count > 0 {
			at(row.item.Category).Shown += row.item.Batch.Count
			continue
		}
		at(row.item.Category).Shown++
	}
	out := make([]crmcontracts.WorklistCount, 0, len(seen))
	for _, count := range seen {
		out = append(out, *count)
	}
	// Ordered by category for the reason reach is ordered by source: two reads
	// of one unchanged day have to produce the same bytes.
	sort.Slice(out, func(i, j int) bool { return out[i].Category < out[j].Category })
	return out
}

// categoryOfSource says which cut of the queue a producer's rows land in.
//
// The classifiers decide this per row, and this is the only other place that
// needs it: to seed a category for a bounded source that contributed no
// surviving row. Two spellings of one fact, and the test beside it feeds every
// source through a classifier and requires the same answer, so a source that
// changes lane fails here rather than reporting its truncation against the
// wrong cut.
func categoryOfSource(source crmcontracts.WorklistItemSource) crmcontracts.WorklistItemCategory {
	switch source {
	case sourceWaiting:
		return "customer_waiting"
	case sourceAtRisk, "brief_item":
		return "deals_at_risk"
	case "meeting":
		return "meetings"
	case sourceTask, "conversation_claim":
		return "tasks"
	case "approval", "dedupe_candidate":
		return "decisions"
	default:
		// Everything the product reports about ITSELF — health, notices,
		// bounces, undelivered mail, failed approvals, privacy deadlines. The
		// default rather than a list, because a source added tomorrow is far
		// more likely to be one of those than a new kind of sales work, and
		// naming it "system" is the answer that under-claims.
		return "system"
	}
}

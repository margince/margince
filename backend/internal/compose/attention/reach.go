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

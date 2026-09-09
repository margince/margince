// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package attention

import (
	"sort"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
)

// pageFrom cuts the candidates to what one read carries, starting at the offset
// a cursor names, and says whether anything is left behind it.
//
// It runs BEFORE the ranking is drawn, so the comparison each row publishes is
// against a row the caller actually received. The cut itself is by score, so
// this sorts first and then slices — taking the best `limit`, never the first
// `limit` the producers happened to return.
//
// The sort happens before the resume, not after, and the order matters: the
// cursor names a position in the RANKING, so the set has to be in ranked order
// before that offset means anything.
//
// It also reports `reached`: where in the ranked set the cut landed, which is
// the offset the next cursor is minted at. That is a position in THIS read's
// ranking, not a running total of rows handed out. The two differ as soon as the
// day moves between pages, and a running total would push the offset past the
// work still owed.
func pageFrom(
	rows []ranked, limit int, cursor worklistCursor,
) (shown []ranked, more bool, reached int) {
	sortByRank(rows)
	from := resumeAt(rows, cursor)
	rows = rows[from:]
	if len(rows) > limit {
		return rows[:limit], true, from + limit
	}
	return rows, false, from + len(rows)
}

// sortByRank puts the day in the order the comparator decides.
//
// Its own function because two pagers now need it and neither may skip it: a
// fresh page cuts this order, and a frozen walk's stored sequence IS a previous
// run of it, so the identities it holds were minted from a ranking rather than
// from whatever the lanes returned.
func sortByRank(rows []ranked) {
	sort.SliceStable(rows, func(i, j int) bool { return less(rows[i], rows[j]) })
}

// The two filter values that are not a category.
//
// Both were added because a Brief surface counted a population this vocabulary
// could not then ask for, so its "N more" link went to a bare queue showing a
// different N. A count and its door now read the same rule.
const (
	// filterExceptDecisions is the complement of `decisions`: what a surface
	// drawing its own decisions deck has not already answered.
	filterExceptDecisions = crmcontracts.WorklistFilter("except_decisions")
	// filterChangedSinceBrief is the rows the overnight run did not see.
	filterChangedSinceBrief = crmcontracts.WorklistFilter("changed_since_brief")
)

// opensTheDeck says whether a filter is a request to see inside the folded
// group of routine decisions.
//
// Only a narrowing that lands ON decisions is. Answering "show me decisions"
// with the group a reader was trying to open is a door back to itself, which is
// the rule foldAndRepin states; answering "show me what changed overnight" with
// a hundred alike rows the unfiltered page draws as one is the same door
// misbehaving the other way.
func opensTheDeck(filter string) bool {
	return filter == string(categoryDecisions)
}

// keepFiltered narrows the candidates to what one filter value asks for.
//
// Most values name a category and are a plain equality. Two are not, and they
// are here rather than in the caller because a filter that lives in two places
// is a filter that answers two things: the surface counting a population and the
// door narrowing to it must read ONE rule, which is the whole reason these two
// values exist.
//
// `changedSinceBrief` decides this one, and it may be absent. A row carries no
// flag when there was no run to compare against or when the row has no material
// moment to date, and absent is refused rather than kept: "the night saw this"
// and "there was no night" are different facts, and answering the whole queue to
// a reader who asked what changed would report a quiet morning as a busy one.
func keepFiltered(rows []ranked, want crmcontracts.WorklistFilter) []ranked {
	kept := make([]ranked, 0, len(rows))
	for _, row := range rows {
		if keepsRow(row, want) {
			kept = append(kept, row)
		}
	}
	return kept
}

func keepsRow(row ranked, want crmcontracts.WorklistFilter) bool {
	switch want {
	case filterExceptDecisions:
		return row.item.Category != categoryDecisions
	case filterChangedSinceBrief:
		// Decisions excluded, and not as a convenience: the strip that counts
		// this population counts it over the rows a decisions-drawing surface is
		// answerable for, so that a row already on screen as a card is not also
		// named as news. A filter admitting decisions would open a longer queue
		// than the number that sent the reader — the defect, one layer down.
		return row.item.Category != categoryDecisions &&
			row.item.ChangedSinceBrief != nil && *row.item.ChangedSinceBrief
	default:
		return string(row.item.Category) == string(want)
	}
}

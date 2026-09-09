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

// opensTheDeck says whether a filter is a request to see inside a folded group.
//
// A narrowing onto a FOLDABLE CATEGORY is. Answering "show me decisions" with
// the group a reader was trying to open is a door back to itself, which is the
// rule foldAndRepin states — and `system` folds too, so the Review verb on a
// broken automation lands here for exactly the same reason. That verb already
// had this bug once: it used to send every group to `decisions`, which filtered
// a system group's own failures out of view.
//
// The two link-only values are NOT that request, which is why this is a set
// rather than `narrowed`. Answering "what changed overnight" with a hundred
// alike rows the unfiltered page draws as one makes the door hold more than the
// count that sent the reader — the same disagreement in the other direction.
func opensTheDeck(filter string) bool {
	return foldableCategories[crmcontracts.WorklistItemCategory(filter)]
}

// The categories batchKeyOf can fold, and therefore the ones a reader can ask
// to see inside. Derived from that function's own branches; the census beside
// it fails if a category learns to fold without arriving here.
var foldableCategories = map[crmcontracts.WorklistItemCategory]bool{
	categoryDecisions: true,
	categorySystem:    true,
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
		return !alreadyACard(row)
	case filterChangedSinceBrief:
		// The deck's own rows excluded, and not as a convenience: the strip that
		// counts this population counts it over the rows a decisions-drawing
		// surface is answerable for, so a row already on screen as a card is not
		// also named as news. A filter admitting them would open a longer queue
		// than the number that sent the reader — the defect, one layer down.
		return !alreadyACard(row) &&
			row.item.ChangedSinceBrief != nil && *row.item.ChangedSinceBrief
	default:
		return string(row.item.Category) == string(want)
	}
}

// alreadyACard says whether a decisions-drawing surface has already answered a
// row, so the complement above must leave it out.
//
// BY SOURCE, matching the client rule exactly. The browser's spelling is
// `item.source !== "approval"` (frontend/src/screens/brief.sentence.ts), and the
// obvious server-side reading — category `decisions` — is a WIDER set: an
// introduction request classifies there and is not an approval, so a category
// test dropped from the door a row the count had included. A count and a door
// that exclude differently is the defect this filter exists to remove, so the
// two sides spell one rule.
//
// A folded group stands for its members and inherits their exclusion; a batch of
// duplicate questions is drawn as a card exactly as its members would be.
func alreadyACard(row ranked) bool {
	if row.item.Source == deckAnswers {
		return true
	}
	for _, folded := range row.foldedFrom {
		if folded == deckAnswers {
			return true
		}
	}
	return false
}

// deckAnswers is the source a brief's decisions deck draws as cards.
//
// The client names the same constant beside its own filter, and the gate below
// holds the pair: two spellings of one exclusion drift the first time either
// side learns a new source.
const deckAnswers = crmcontracts.WorklistItemSource("approval")

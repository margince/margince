// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package attention

// One deal is ONE row, however many lanes found it.
//
// The brief ranks deals overnight and the at-risk lane raises them during the
// day, and the two agree far more often than they disagree — that is what makes
// them both right, and what put the same deal on the morning twice: the same
// name, the same figures, two places to answer it, and two rows counted in every
// reading above them.
//
// THE AT-RISK ROW SURVIVES, NOT THE BRIEF ROW. It is the one with the day's
// facts on it: the figures pass filled its amount and close date, the classifier
// gave it a level from those facts, and the move pass may have put a next step on
// it. The brief row carries a rank and a deal id. Folding the other way would
// keep the poorer row and lose the reason it was ranked where it was.
//
// WHAT THE BRIEF ROW LEAVES BEHIND is its id, on `brief_item_id`. The brief's
// own verbs — act, set aside, dismiss — route to `/brief/items/{id}/…`, and a
// reader answering the surviving row must still be able to tell the night it was
// answered. Without the id the fold would silently take those verbs away, which
// is a worse outcome than the duplicate.
//
// AND ITS COMPOSITE, which is what makes the fold more than tidying. The night's
// score is the only ordering signal that knows how this deal compares to every
// other deal the night considered, and ranksteps.go's `opportunity` step reads it
// to break a tie INSIDE a level. Dropping the row without it would throw that
// away.
//
// WHAT THE FOLD MUST NOT CHANGE is the surviving row's level, its band, or any
// count derived from them. semanticLevelOf has three readers — the summary's
// signals, bucketsOf's partition, and the partition a walk freezes — and its own
// comment warns that a fourth spelling of that precedence is the defect to
// avoid. So this touches none of them: it removes a row and copies two fields
// onto another, and TestFoldingABriefItemMovesNoBucketCount holds that.

import (
	"time"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// foldBriefIntoRisk drops each brief row whose deal another row already reports,
// carrying the brief's id and its composite onto the row that survives.
//
// Rows the fold does not touch keep their order and their contents exactly, so
// this is safe to call before the ranking: it is a filter, not a re-sort.
func foldBriefIntoRisk(rows []ranked) []ranked {
	kept := make([]ranked, 0, len(rows))
	// Where each surviving row ended up in KEPT, not in rows. The brief entries
	// are dropped as the walk goes, so the two indexes diverge the moment one
	// is — and writing the brief's id at the row's old position would put it on
	// whatever row slid into that slot, or past the end.
	survivingAt := map[ids.UUID]int{}
	folded := make([]ranked, 0, 4)
	for _, row := range rows {
		if _, isBrief := briefRowsDeal(row.item); isBrief {
			folded = append(folded, row)
			continue
		}
		if row.item.Subject != nil && row.item.Subject.Type == subjectDeal {
			deal := ids.UUID(row.item.Subject.Id)
			// The FIRST row about a deal wins: it is the one the ranking shows
			// highest, and the brief's verbs must land where the reader looks.
			if _, seen := survivingAt[deal]; !seen {
				survivingAt[deal] = len(kept)
			}
		}
		kept = append(kept, row)
	}
	for _, brief := range folded {
		deal, _ := briefRowsDeal(brief.item)
		at, found := survivingAt[deal]
		if !found {
			// The night surfaced a deal the day's own lanes did not. Its row
			// stays as itself — this fold removes duplicates and never rows.
			kept = append(kept, brief)
			continue
		}
		carryBriefOnto(&kept[at], brief)
	}
	return kept
}

// briefRowsDeal answers which deal a brief row is about, where the row is one.
func briefRowsDeal(item crmcontracts.WorklistItem) (ids.UUID, bool) {
	if item.Source != crmcontracts.WorklistItemSourceBriefItem {
		return ids.UUID{}, false
	}
	if item.Subject == nil || item.Subject.Type != subjectDeal {
		return ids.UUID{}, false
	}
	return ids.UUID(item.Subject.Id), true
}

// carryBriefOnto moves what the folded row was carrying onto the survivor: the
// brief's own id, so its verbs still reach it, and the night's composite, so the
// opportunity tie-break can still read it.
//
// The survivor's level, band and every other field are untouched, which is the
// invariant this fold is allowed to keep and nothing more.
func carryBriefOnto(survivor *ranked, brief ranked) {
	id, err := ids.Parse(brief.item.Id)
	if err != nil {
		// A brief row whose own id will not parse is a row whose verbs could not
		// be addressed anyway. The duplicate is still folded away; what is lost
		// is the routing, which was already lost.
		return
	}
	named := openapi_types.UUID(id)
	survivor.item.BriefItemId = &named
	survivor.opportunity = brief.opportunity
}

// stampOpportunity puts the night's score on every row about a deal it ranked.
//
// EVERY row, not only the brief's own: the fold below keeps the at-risk row, and
// a score reaching only the row that disappears would be a tie-break nothing
// could read. A deal the night never saw keeps zero and loses the step to any
// deal it did, which is the honest answer — the night has an opinion about one
// and none about the other.
func stampOpportunity(rows []ranked, scores map[ids.UUID]float64) []ranked {
	if len(scores) == 0 {
		return rows
	}
	for at := range rows {
		item := rows[at].item
		if item.Subject == nil || item.Subject.Type != subjectDeal {
			continue
		}
		if score, ranked := scores[ids.UUID(item.Subject.Id)]; ranked {
			rows[at].opportunity = score
		}
	}
	return rows
}

// markChangedSinceBrief says which rows report something the night did not see.
//
// AGAINST THE RUN'S DATA CUTOFF, never its generated_at. A run written at 06:42
// over records read at 06:00 has a 42-minute window, and judging freshness by
// when the rows were written would call every one of them old — hiding exactly
// the reply a rep opens the page to find.
//
// The row's own material moment is what is compared: `occurredAt`, which each
// producer already set to the instant the thing it reports actually happened.
// The alternative — the browser comparing some timestamp on the wire — is a
// second answer to "is this new", in a place that cannot see which of a row's
// several instants is the material one.
//
// NO RUN MEANS NO ANSWER, not a false one. A zero cutoff leaves every row's flag
// absent, because "the night saw this" and "there was no night" are different
// facts and a client showing a changed-since strip must be able to tell them
// apart.
func markChangedSinceBrief(rows []ranked, cutoff time.Time) []ranked {
	if cutoff.IsZero() {
		return rows
	}
	for at := range rows {
		if rows[at].occurredAt.IsZero() {
			// A row with no material moment cannot answer this question. Absent
			// rather than false: false would claim the night saw something the
			// row cannot date.
			continue
		}
		changed := rows[at].occurredAt.After(cutoff)
		rows[at].item.ChangedSinceBrief = &changed
	}
	return rows
}

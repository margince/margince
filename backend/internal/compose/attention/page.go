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
	sort.SliceStable(rows, func(i, j int) bool { return less(rows[i], rows[j]) })
	from := resumeAt(rows, cursor)
	rows = rows[from:]
	if len(rows) > limit {
		return rows[:limit], true, from + limit
	}
	return rows, false, from + len(rows)
}

func keepCategory(rows []ranked, want crmcontracts.WorklistItemCategory) []ranked {
	kept := make([]ranked, 0, len(rows))
	for _, row := range rows {
		if row.item.Category == want {
			kept = append(kept, row)
		}
	}
	return kept
}

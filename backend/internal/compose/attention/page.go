// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package attention

import (
	"sort"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
)

// pageFrom cuts the candidates to what one read carries, starting after the row
// a cursor names, and says whether anything is left behind it.
//
// It runs BEFORE the ranking is drawn, so the comparison each row publishes is
// against a row the caller actually received. The cut itself is by score, so
// this sorts first and then slices — taking the best `limit`, never the first
// `limit` the producers happened to return.
//
// The sort happens before the resume, not after, and the order matters: the
// cursor names a position in the RANKING, so the set has to be in ranked order
// before the anchor means anything.
func pageFrom(rows []ranked, limit int, cursor worklistCursor) (shown []ranked, more bool) {
	sort.SliceStable(rows, func(i, j int) bool { return less(rows[i], rows[j]) })
	rows = resume(rows, cursor)
	if len(rows) > limit {
		return rows[:limit], true
	}
	return rows, false
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

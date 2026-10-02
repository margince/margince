// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package search

import (
	"fmt"
	"strings"
)

// maxPerType bounds a grouped page. Every union element at this cap, the
// employer arm included, stays within the largest ranked page, so a grouped
// answer never costs more than a ranked one
// (TestAGroupedPageFitsInTheLargestRankedPage).
const maxPerType = 20

// groupedShape is a few hits of EVERY type that matched, each type's best
// first. ts_rank_cd does not compare across types (see Hit), so a short ranked
// list can hold nothing but mail; ranking and capping inside each type lets
// every type a word found show.
type groupedShape struct {
	perType int
}

// groupedShapeFor validates a grouped request. It answers one page, whole, so
// a cursor or a limit alongside it would be a page of a different answer.
func groupedShapeFor(perType int, in Input) (groupedShape, error) {
	if perType < 1 || perType > maxPerType {
		return groupedShape{}, &BadQueryError{Field: "per_type", Reason: fmt.Sprintf("per_type must be between 1 and %d", maxPerType)}
	}
	if in.Cursor != "" || in.Limit != 0 {
		return groupedShape{}, &BadQueryError{Field: "per_type", Reason: "per_type answers one grouped page and takes no cursor or limit — " +
			"drop per_type and narrow with types to page through one type"}
	}
	return groupedShape{perType: perType}, nil
}

// statement caps each branch on its own, before the union, so one type's
// matches cannot crowd another's out. The cap is one past perType: the extra
// row is how page tells a type that holds more from one that was exhausted.
func (g groupedShape) statement(branches []string, within string, arg func(any) int) string {
	capPos := arg(g.perType + 1)
	capped := make([]string, len(branches))
	for i, branch := range branches {
		sql := "SELECT " + hitColumns + " FROM (" + branch + ") b"
		if within != "" {
			sql += " WHERE " + within
		}
		capped[i] = fmt.Sprintf("(%s ORDER BY score DESC, id LIMIT $%d)", sql, capPos)
	}
	// Type first, so each type's hits arrive together: a score is no reason
	// to interleave two types that it cannot compare.
	return "SELECT " + hitColumns + " FROM (" + strings.Join(capped, " UNION ALL ") +
		") grouped ORDER BY rtype, score DESC, id"
}

// page drops each type's overfetched row and names the types it came from.
// Each type's rows arrive best first, so the one dropped is its worst.
func (g groupedShape) page(hits []Hit) Page {
	seen := map[string]int{}
	more := map[string]bool{}
	// Non-nil even when nothing was cut: an empty list on a grouped page says
	// every type is whole, where nil says the page was never grouped.
	page := Page{Hits: make([]Hit, 0, len(hits)), TypesWithMore: []string{}}
	for _, hit := range hits {
		seen[hit.Type]++
		if seen[hit.Type] > g.perType {
			more[hit.Type] = true
			continue
		}
		page.Hits = append(page.Hits, hit)
	}
	for _, branch := range searchBranches {
		if more[branch.entity] {
			page.TypesWithMore = append(page.TypesWithMore, branch.entity)
		}
	}
	return page
}

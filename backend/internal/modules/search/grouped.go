// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package search

import (
	"fmt"
	"math"
	"slices"
	"strings"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
)

// maxPerType bounds one type's share of a grouped page. Every union element at
// this cap can exceed the largest ranked page, so page lowers the share until the
// whole answer fits (TestAGroupedPageFitsInTheLargestRankedPage).
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
//
// A branch with own rows reads them first. They may fill the cap above every
// score the rest of the branch can reach. Then the rest and its fallback arms
// never run, because Postgres plans the filled check as a constant.
func (g groupedShape) statement(branches []unionBranch, within string, arg func(any) int) string {
	capPos := arg(g.perType + 1)
	capped := func(sql string, conditions ...string) string {
		where := strings.Join(slices.DeleteFunc(append([]string{within}, conditions...), func(c string) bool { return c == "" }), " AND ")
		if where != "" {
			where = " WHERE " + where
		}
		return fmt.Sprintf("(SELECT %s FROM (%s) b%s ORDER BY score DESC, id LIMIT $%d)", hitColumns, sql, where, capPos)
	}
	var owned []string
	filled := map[string]string{}
	for _, branch := range branches {
		if branch.own == "" {
			continue
		}
		name := "own_" + branch.entity
		owned = append(owned, name+" AS MATERIALIZED "+capped(branch.own))
		filled[branch.entity] = fmt.Sprintf("((SELECT count(*) FROM %s WHERE score > $%d) >= $%d)", name, arg(branch.unrankedCeiling), capPos)
	}
	var elements []string
	for _, branch := range branches {
		switch {
		case branch.own != "":
			elements = append(elements, fmt.Sprintf("(SELECT %s FROM own_%s WHERE %s)", hitColumns, branch.entity, filled[branch.entity]),
				capped(branch.sql, "NOT "+filled[branch.entity]))
		case filled[branch.fallbackOf] != "":
			elements = append(elements, capped(branch.sql, "NOT "+filled[branch.fallbackOf]))
		default:
			elements = append(elements, capped(branch.sql))
		}
	}
	// Type first, so each type's hits arrive together: a score is no reason
	// to interleave two types that it cannot compare.
	statement := "SELECT " + hitColumns + " FROM (" + strings.Join(elements, " UNION ALL ") +
		") grouped ORDER BY rtype, score DESC, id"
	if len(owned) == 0 {
		return statement
	}
	return "WITH " + strings.Join(owned, ", ") + " " + statement
}

// page drops each type's overfetched row and names the types it came from.
// Each type's rows arrive best first, so the one dropped is its worst.
func (g groupedShape) page(hits []Hit) Page { return g.pageWith(hits, nil) }

// pageWith is page for a page whose timedOut types ran out of time and hold no
// hits.
func (g groupedShape) pageWith(hits []Hit, timedOut []string) Page {
	perType := g.fittingPerType(hits)
	seen := map[string]int{}
	more := map[string]bool{}
	// Non-nil even when nothing was cut: an empty list on a grouped page says
	// every type is whole, where nil says the page was never grouped.
	page := Page{Hits: make([]Hit, 0, len(hits)), TypesWithMore: []string{}, TypesTimedOut: []string{}}
	for _, hit := range hits {
		seen[hit.Type]++
		if seen[hit.Type] > perType {
			more[hit.Type] = true
			continue
		}
		page.Hits = append(page.Hits, hit)
	}
	// One row per type can still pass the page limit when more than that many
	// types matched. The rows arrive type by type, so the cut falls on the last.
	if limit := rankedPageLimit(); len(page.Hits) > limit {
		for _, hit := range page.Hits[limit:] {
			more[hit.Type] = true
		}
		page.Hits = page.Hits[:limit]
	}
	page.TypesWithMore = typesIn(more)
	page.TypesTimedOut = typesIn(timedOutSet(timedOut))
	return page
}

// fittingPerType is the largest share, at most perType, at which every type's
// best rows together stay within the largest ranked page. Each type's rows arrive
// best first, so lowering the share trims every type's worst rows alike.
func (g groupedShape) fittingPerType(hits []Hit) int {
	largest := rankedPageLimit()
	for share := g.perType; share > 1; share-- {
		seen := map[string]int{}
		kept := 0
		for _, hit := range hits {
			seen[hit.Type]++
			if seen[hit.Type] <= share {
				kept++
			}
		}
		if kept <= largest {
			return share
		}
	}
	return 1
}

// rankedPageLimit is the largest page a ranked search answers.
func rankedPageLimit() int {
	unbounded := math.MaxInt
	return storekit.ClampLimit(&unbounded)
}

func timedOutSet(types []string) map[string]bool {
	set := make(map[string]bool, len(types))
	for _, t := range types {
		set[t] = true
	}
	return set
}

// typesIn lists the members of set in one order everywhere: core types in
// searchBranches order, then declared kinds by name. Never nil.
func typesIn(set map[string]bool) []string {
	types := []string{}
	for _, branch := range searchBranches {
		if set[branch.entity] {
			types = append(types, branch.entity)
		}
	}
	for _, kind := range extensionTypes() {
		if set[kind] {
			types = append(types, kind)
		}
	}
	return types
}

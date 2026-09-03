// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package analyticsquery

// The group floor: how few records an answer may describe.
//
// TWO RULES, and the second is the one that is usually missed.
//
// First: a group with fewer rows than the floor is withheld. A breakdown by
// owner where one owner has two deals is a report about two deals, and over a
// small enough group an average IS the value.
//
// Second: WITHHOLDING ONE GROUP IS NOT ENOUGH. If the other groups and the
// total are both shown, the withheld group is the difference — the reader
// subtracts and has it exactly. So when any group is withheld, the smallest
// remaining group goes too, and the total is withheld with them. That costs a
// real answer and is the only thing that actually holds: a suppression somebody
// can undo with arithmetic is a suppression in name only.
//
// And below the floor no measure is served but the count. A median over four
// rows is close to one row's value; a sum over two names both amounts to
// anybody who knows one.
//
// WHAT THIS FILE DOES NOT DO: it judges one answer. Two answers that differ by
// a filter can be subtracted, and no amount of care inside a single response
// prevents that — refuseIfTheFilterHidesTooLittle in analyticsqueryrun.go is
// what closes it, by counting what a filter removed before serving the answer.

import "sort"

// Floor is how few rows a group may cover and still be reported.
type Floor int

// DefaultFloor is the floor when the installation has not set one.
//
// Five, matching the sample floor the forecast's percentiles already use
// (compose/report.go). One number across the product rather than two: a reader
// who learns that a figure is withheld below five should not find a different
// five somewhere else.
const DefaultFloor Floor = 5

// Row is one group of an answer.
type Row struct {
	// Keys are the group key values, in the plan's column order.
	Keys []any
	// Count is how many rows the group covers — the plan's own count, not a
	// measure the caller asked for.
	Count int
	// Values are the caller's measures, in order.
	Values []any
	// Withheld says the group was suppressed. The row is still RETURNED, with
	// NOTHING in it — no keys and no numbers. Dropping it entirely would make
	// the answer's row count a signal of its own; keeping the keys turned a
	// grouping by a high-cardinality field into a paginated dump of every
	// record's identity, which is the half that actually needed protecting.
	Withheld bool
}

// Apply withholds what the floor covers, INCLUDING the complements.
//
// Returns the rows with small groups blanked, and whether anything was
// withheld. The caller reports that boolean; it never reports how many.
func (f Floor) Apply(rows []Row) ([]Row, bool) {
	if f <= 0 {
		return rows, false
	}
	below := 0
	for _, row := range rows {
		if row.Count < int(f) {
			below++
		}
	}
	if below == 0 {
		return rows, false
	}

	// The complement pass. Withholding the small groups alone leaves each of
	// them equal to the total minus the rest, so enough of the remainder goes
	// with them that no single withheld group is recoverable.
	//
	// "Enough" is one more group than were withheld, taken from the smallest
	// remaining. With two or more unknowns in the equation the reader has a
	// sum and not a value — and the smallest are chosen because they cost the
	// least information to lose.
	order := make([]int, len(rows))
	for i := range order {
		order[i] = i
	}
	sort.SliceStable(order, func(a, b int) bool {
		return rows[order[a]].Count < rows[order[b]].Count
	})

	suppress := map[int]bool{}
	for _, idx := range order {
		if rows[idx].Count < int(f) {
			suppress[idx] = true
		}
	}
	// One more, so the withheld set has at least two members and no member is
	// solvable on its own.
	for _, idx := range order {
		if len(suppress) > below {
			break
		}
		suppress[idx] = true
	}

	out := make([]Row, len(rows))
	copy(out, rows)
	for idx := range suppress {
		out[idx].Withheld = true
		out[idx].Values = nil
		// The count goes too. It is the measure the floor is judged on, and
		// leaving it would report the size of a group whose numbers were
		// withheld for being that size.
		out[idx].Count = 0
	}
	return out, true
}

// TotalIsSafe answers whether an answer's TOTAL may be reported alongside these
// rows.
//
// It may not, once anything is withheld. The total plus every shown group is
// the withheld remainder, which is the subtraction this floor exists to stop —
// and it is the one a reader performs without meaning to, because a total is
// the first thing they add up against.
func TotalIsSafe(withheld bool) bool { return !withheld }

// AllowsMeasure answers whether a measure may be served for a group at this
// size.
//
// Below the floor, only the count — and by the time this is asked the count
// itself has been withheld, so in practice this refuses every measure. It is
// stated as its own rule because the reason differs: the count is withheld to
// stop the subtraction, and the others are withheld because over four rows an
// average is a value.
func (f Floor) AllowsMeasure(fn AggFn, count int) bool {
	if count >= int(f) {
		return true
	}
	return fn == CountAll
}

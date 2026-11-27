// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package deals

import (
	"slices"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func offerLinesAt(positions ...int) []ranked {
	out := make([]ranked, len(positions))
	for i, p := range positions {
		out[i] = ranked{id: ids.NewV7(), position: p}
	}
	return out
}

// applyRowByRow writes one statement's targets a row at a time, in the given
// order. It refuses a write onto a position another row holds at that moment,
// which is how Postgres checks a unique index that is not deferrable.
func applyRowByRow(t *testing.T, held map[ids.UUID]int, movers []ids.UUID, targets []int, reverse bool) {
	t.Helper()
	index := make([]int, len(movers))
	for i := range index {
		index[i] = i
	}
	if reverse {
		slices.Reverse(index)
	}
	for _, i := range index {
		for other, at := range held {
			if other != movers[i] && at == targets[i] {
				t.Fatalf("writing position %d collides with a line that holds it", targets[i])
			}
		}
		if targets[i] < 1 {
			t.Fatalf("position %d breaks the floor of 1", targets[i])
		}
		held[movers[i]] = targets[i]
	}
}

func TestAMovedLineTakesItsPlaceAndTheOthersCloseUp(t *testing.T) {
	cases := map[string]struct {
		positions []int
		move, to  int
		want      []int
	}{
		"the second line to the top":       {[]int{1, 2}, 1, 1, []int{1, 0}},
		"the first line down one":          {[]int{1, 2, 3}, 0, 2, []int{1, 0, 2}},
		"the last line to the top":         {[]int{1, 2, 3}, 2, 1, []int{2, 0, 1}},
		"past the last line goes last":     {[]int{1, 2, 3}, 0, 99, []int{1, 2, 0}},
		"gapped positions close up":        {[]int{2, 5, 9}, 2, 2, []int{0, 2, 1}},
		"a high position is parked safely": {[]int{1, 4, 5}, 0, 3, []int{1, 2, 0}},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			for _, reverse := range []bool{false, true} {
				lines := offerLinesAt(c.positions...)
				moves, parks := lineMovePlan(lines, lines[c.move].id, c.to)
				held := make(map[ids.UUID]int, len(lines))
				for _, r := range lines {
					held[r.id] = r.position
				}
				movers := make([]ids.UUID, len(moves))
				ranks := make([]int, len(moves))
				for i, m := range moves {
					movers[i], ranks[i] = m.id, m.to
				}
				applyRowByRow(t, held, movers, parks, reverse)
				applyRowByRow(t, held, movers, ranks, reverse)
				for rank, at := range c.want {
					if got := held[lines[at].id]; got != rank+1 {
						t.Errorf("line %d sits at %d, want %d", at, got, rank+1)
					}
				}
			}
		})
	}
}

func TestAMoveToItsOwnPlaceWritesNothing(t *testing.T) {
	lines := offerLinesAt(1, 2, 3)
	if moves, parks := lineMovePlan(lines, lines[1].id, 2); len(moves) != 0 || len(parks) != 0 {
		t.Fatalf("a line moved to the place it holds wrote %v via %v", moves, parks)
	}
	if moves, _ := lineMovePlan(lines, ids.NewV7(), 1); len(moves) != 0 {
		t.Fatalf("a line the offer does not hold moved %v", moves)
	}
}

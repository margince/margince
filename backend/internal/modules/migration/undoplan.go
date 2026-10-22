// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package migration

import (
	"cmp"
	"context"
	"fmt"
	"slices"

	"github.com/jackc/pgx/v5"
)

// undoPlan is how many created rows an undo reverses per object, in the order
// it walks them. The checkpoint counts rows across that order, so the rows
// done per object follow from the checkpoint and the totals alone.
type undoPlan []ObjectProgress

// undoPlanOf counts the rows the run created, per object, later types first.
func undoPlanOf(ctx context.Context, tx pgx.Tx, id RunID) (undoPlan, error) {
	r, err := tx.Query(ctx, `
		SELECT object, count(*) FROM import_record_map
		 WHERE import_run_id = $1 AND origin = 'created'
		 GROUP BY object`, id)
	if err != nil {
		return nil, fmt.Errorf("counting import run %s's created rows: %w", id, err)
	}
	defer r.Close()
	var plan undoPlan
	for r.Next() {
		var one ObjectProgress
		if err := r.Scan(&one.Object, &one.RowsTotal); err != nil {
			return nil, fmt.Errorf("counting import run %s's created rows: %w", id, err)
		}
		plan = append(plan, one)
	}
	if err := r.Err(); err != nil {
		return nil, fmt.Errorf("counting import run %s's created rows: %w", id, err)
	}
	// The same order mapRowsForRun walks: a type outside the list goes last.
	order := reverseImportOrder()
	rank := func(object string) int {
		if i := slices.Index(order, object); i >= 0 {
			return i
		}
		return len(order)
	}
	slices.SortFunc(plan, func(a, b ObjectProgress) int {
		return cmp.Or(cmp.Compare(rank(a.Object), rank(b.Object)), cmp.Compare(a.Object, b.Object))
	})
	return plan, nil
}

// at is the progress once processed rows are reversed, and the object the
// walk is on. An undo is done with every object when the second is "".
func (p undoPlan) at(processed int) (Progress, string) {
	out := Progress{Objects: make([]ObjectProgress, 0, len(p))}
	current := ""
	left := processed
	for _, one := range p {
		done := min(max(left, 0), one.RowsTotal)
		left -= done
		one.RowsDone = done
		if current == "" && done < one.RowsTotal {
			current = one.Object
		}
		out.Objects = append(out.Objects, one)
	}
	return out, current
}

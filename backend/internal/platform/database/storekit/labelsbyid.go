// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package storekit

// The display-name batch read, once.
//
// Several modules answer "the names of these records" for the attention
// feed's label pass, and they differ only in which table, which column, and
// which scope clause — the parts that are genuinely each module's own. What
// they share is the part that must not drift: an empty name is ABSENT rather
// than a blank string, because a card showing nothing where a record should
// be says more than one showing "".
//
// The statement is the caller's, already carrying its own grant and row
// scope; this holds no authority and adds no predicate.

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// LabelsByID runs one already-scoped statement of the shape
// `SELECT id, coalesce(<name>, ”) …` and collects the rows that carry a
// name.
//
// A row the statement did not return is absent for whatever reason the
// caller's own clauses decided — gone, archived, out of scope, withheld —
// and this cannot tell those apart, which is the point: everywhere the label
// pass speaks, they are the same answer.
func LabelsByID(ctx context.Context, tx pgx.Tx, statement string, args ...any) (map[ids.UUID]string, error) {
	labels := map[ids.UUID]string{}
	rows, err := tx.Query(ctx, statement, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var id ids.UUID
		var label string
		if err := rows.Scan(&id, &label); err != nil {
			return nil, err
		}
		if label != "" {
			labels[id] = label
		}
	}
	return labels, rows.Err()
}

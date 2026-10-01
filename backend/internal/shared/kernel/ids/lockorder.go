// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ids

import (
	"bytes"
	"slices"
)

// Ascending is the order several rows are locked in.
//
// A transaction that takes row locks on more than one record of a kind must
// take them in an order every other writer agrees on, or two transactions
// holding each other's next row deadlock and Postgres aborts one of them. The
// id is the only ordering every writer already has: it needs no read, it is
// stable, and it does not depend on which query found the rows.
//
// Which order does not matter as long as every writer takes the same one. Ascending is chosen because
// it is what a reader assumes on seeing a sort, and because `ORDER BY id` says
// the same thing in SQL for a writer that locks in one statement.
//
// Answers a COPY. A caller's slice is often the result set it also reports on,
// and reordering that under it would change what a count or a first-element
// read means.
func Ascending[K EntityKind](in []ID[K]) []ID[K] {
	out := slices.Clone(in)
	slices.SortFunc(out, func(a, b ID[K]) int {
		return bytes.Compare(a.UUID[:], b.UUID[:])
	})
	return out
}

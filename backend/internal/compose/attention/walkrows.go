// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package attention

// The identities a walk freezes and later compares the live day against.

import "github.com/margince/margince/backend/internal/compose/worklistsnap"

// countArrived is how many items of this read the walk never carried. A group
// is judged by its members, because the walk froze them and not the group's id.
func countArrived(rows []ranked, carried map[worklistsnap.Row]bool) int {
	arrived := 0
	for _, row := range rows {
		if len(row.members) == 0 {
			if !carried[rowIdentity(row)] {
				arrived++
			}
			continue
		}
		for _, member := range row.members {
			if !carried[member] {
				arrived++
			}
		}
	}
	return arrived
}

// rowIdentity names a row the way the walk and the pin store key it. Lanes mint
// ids independently, so the source is part of the identity.
func rowIdentity(row ranked) worklistsnap.Row {
	return worklistsnap.Row{Source: string(row.item.Source), RowID: row.item.Id}
}

// frozenRows is the identity list a first page hands the snapshot store.
//
// Order matters and content does not: this is what the walk will be replayed
// in, and every row's text is re-read live on each page.
func frozenRows(rows []ranked) []worklistsnap.Row {
	out := make([]worklistsnap.Row, 0, len(rows))
	for _, row := range rows {
		// A group freezes its members. Its own id exists only while the fold
		// mints it, so the members are what a later page can still find.
		if len(row.members) > 0 {
			out = append(out, row.members...)
			continue
		}
		out = append(out, rowIdentity(row))
	}
	return out
}

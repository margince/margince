// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import "sort"

// CoalesceGuardedColumns names, per record type, the columns that record type's
// update path writes as `coalesce($n, col)`. The placeholder's NULL selects the
// current value, so an explicit NULL through that path cannot clear the column:
// the write succeeds and changes nothing.
//
// A reversal has to know this. Restoring a before-image that holds NULL for such
// a column would report success and leave the old value standing — a dishonest
// success, which is the one outcome worse than a refusal, because the human
// reads the confirmation and stops looking.
//
// This is a claim about SQL in six modules, and nullableclearing_test.go holds
// it equal to that SQL. A hand-kept list would fail short in silence the moment
// a column was added, and a census that can fail short has already failed.
var coalesceGuardedColumns = map[string][]string{
	// The other five record types patch through storekit.Patch, which writes
	// only the columns the caller supplied and can therefore write NULL. An
	// empty answer for them is a result, not an omission.
	"activity": {
		"assignee_id", "body", "due_at", "is_done",
		"occurred_at", "remind_at", "subject",
	},
}

// CoalesceGuardedColumns reports the columns a restore of recordType cannot set
// to NULL, sorted. An unknown record type has none, which is the honest answer:
// this map covers the types the reversal path serves, and a type it does not
// serve is refused earlier for not being one.
func CoalesceGuardedColumns(recordType string) []string {
	declared := coalesceGuardedColumns[recordType]
	out := append([]string(nil), declared...)
	sort.Strings(out)
	return out
}

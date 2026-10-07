// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package storekit

// CitedActivityID renders the activity an evidence citation names, as a uuid,
// so a reader joins activity through its primary key rather than by text.
//
// item is a SQL expression for one element of an evidence array — a jsonb
// object carrying source_type and source_id. Comparing text instead
// (`a.id::text = item->>'source_id'`) cannot use any index: it renders every
// candidate activity id as text and compares them one by one, so the cost of a
// citation check grows with the size of the activity table rather than with
// the number of citations.
//
// The pattern admits exactly the ids that text comparison could match and no
// other. A uuid's text form is its canonical lowercase spelling, which is also
// how evidence writers render source_id; any other spelling could never have
// equalled a uuid's text and here reads as NULL, joining nothing. The CASE is
// what keeps the cast from ever seeing such a value: an unguarded cast would
// raise on the first one and fail the whole statement.
func CitedActivityID(item string) string {
	return "(CASE WHEN (" + item + "->>'source_id') ~ " +
		"'^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$' " +
		"THEN (" + item + "->>'source_id')::uuid END)"
}

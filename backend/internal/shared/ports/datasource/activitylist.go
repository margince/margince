// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package datasource

// ActivityListScope is how one activity list is narrowed, as far as deciding
// whether it reads as the reader's diary. GET /activities and list_records
// each fill it from their own filters and ask compose the same question.
type ActivityListScope struct {
	Kind string
	// Windowed is true when the list has an occurred_after or occurred_before bound.
	Windowed bool
	// Narrowed is true when the list is held to one record, project or assignee.
	Narrowed bool
}

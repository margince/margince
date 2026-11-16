// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import "fmt"

// BoundActivityPage selects one page of activity ids and joins the rows back
// by id. Per-row work added after it, such as the content decision, then runs
// only for the page. It names only the id: activity gains custom-field columns
// at runtime, so a whole-row subquery is not allowed (gates/runtimeddlwholerow).
func BoundActivityPage(source, where, orderBy string, limit int) string {
	return "(SELECT a.id FROM " + source + " WHERE " + where + orderBy +
		fmt.Sprintf(" LIMIT %d", limit) + ") bounded JOIN activity a ON a.id = bounded.id"
}

// ActivityContentFrom evaluates a content decision once for each source row.
// Callers must bound paged sources first; this fence must not precede their limit.
func ActivityContentFrom(from, content string) (string, string) {
	switch content {
	case sqlTrue, scopeUnbounded, SeatCopyReadable:
		return from, content
	default:
		return from + " CROSS JOIN LATERAL (SELECT (" + content +
			") AS available OFFSET 0) content_decision", "content_decision.available"
	}
}

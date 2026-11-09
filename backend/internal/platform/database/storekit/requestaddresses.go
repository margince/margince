// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package storekit

// RequestAddressesSQL lists every live address for consent, acquisition and dedupe.
// The fence keeps the archive predicate from selecting a domain-wide partial index.
func RequestAddressesSQL(column string) string {
	return "(SELECT coalesce(array_agg(ce.email ORDER BY ce.email), '{}') FROM " +
		"(SELECT email, archived_at FROM contact_email WHERE contact_id = " + column +
		" OFFSET 0) ce WHERE ce.archived_at IS NULL)"
}

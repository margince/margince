// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package storekit

// RequestAddressesSQL is the list a contact merge binds a request to: the live
// addresses of the contact named by column, never NULL. Contacts writes it on
// the acquisition evidence and consent on the qualifying events, each on its
// own table, so the expression lives here once.
func RequestAddressesSQL(column string) string {
	return "(SELECT coalesce(array_agg(ce.email ORDER BY ce.email), '{}') FROM contact_email ce " +
		"WHERE ce.contact_id = " + column + " AND ce.archived_at IS NULL)"
}

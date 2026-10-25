// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package relstrength

import "fmt"

// SenderSQL renders "this contact wrote this message", not merely "it reached
// them".
//
// A thread is linked to everybody it concerns, so activity_link membership says
// nothing about authorship. Reading it that way put another sender's title,
// employer and address onto a contact.
//
// The address decides whenever the row carries one. The backfill for historical
// mail takes contact_id from the first activity_link and the address from
// counterparty_email. One row can then name one contact and carry another's
// address. contact_id is read only for a row with no address, the shape live
// capture writes when it resolved a party by channel account alone.
//
// A message with no from participant is not admitted through
// activity.counterparty_email. Those rows are machine notifications, and a
// fallback would hand their footers to whichever contact they named.
//
// contact and activity are SQL expressions the caller supplies, such as a bind
// parameter ("$3") or a correlated column ("p.id"), and the activity's alias.
// It is a format string, not a builder function, because the census of
// unbounded record references reads one function at a time.
func SenderSQL(contact, activity string) string {
	return fmt.Sprintf(senderSQL, contact, activity)
}

const senderSQL = `EXISTS (
		SELECT 1 FROM activity_participant ap
		 WHERE ap.activity_id = %[2]s.id AND ap.role = 'from'
		   AND CASE WHEN ap.address IS NOT NULL
		            THEN EXISTS (SELECT 1 FROM contact_email pe
		                          WHERE pe.contact_id = %[1]s AND pe.archived_at IS NULL
		                            AND pe.email = ap.address)
		            ELSE ap.contact_id = %[1]s END)`

// ContactExchangeSQL admits an activity to the contact's last-touch clock: an
// exchange that has happened, and, when inbound, one the contact wrote. A
// colleague's message on a thread the contact only belongs to is not heard
// from them. The reconnect lane and the colleague edge's last_at both read it.
func ContactExchangeSQL(activity, contact, now string) string {
	return InteractionCountsSQL(activity) + " AND " + LastActivityCountsSQL(activity, now) +
		" AND (" + activity + ".direction IS DISTINCT FROM 'inbound' OR " + SenderSQL(contact, activity) + ")"
}

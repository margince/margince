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
// The stored contact decides in three cases. The row has no address. The
// contact holds the row's address. Or they once held it, and another contact
// holds it now. So mail stays with its author after the address moves on, but an
// address removed from a contact no longer speaks for them. Otherwise the
// address decides only where it proves the contact (AddressProvesSQL), so a
// moved address names nobody. The backfill
// took contact_id from the first link and the address from counterparty_email,
// so one row can name one contact and carry another's address.
//
// A message with no from participant is not admitted through
// activity.counterparty_email. Those rows are machine notifications, and a
// fallback would hand their footers to whichever contact they named.
//
// contact and activity are SQL expressions the caller supplies, such as a bind
// parameter ("$3") or a correlated column ("p.id"), and the activity's alias.
func SenderSQL(contact, activity string) string {
	return fmt.Sprintf(senderSQL, contact, activity, ParticipantNamesSQL(contact, "ap"))
}

const senderSQL = `EXISTS (
		SELECT 1 FROM activity_participant ap
		 WHERE ap.activity_id = %[2]s.id AND ap.role = 'from'
		   AND %[3]s)`

// ParticipantNamesSQL renders "participant row `participant` names contact
// `contact`", by the rule SenderSQL states. Every reader that asks which part a
// contact had on a message asks it through this one rule.
func ParticipantNamesSQL(contact, participant string) string {
	return fmt.Sprintf(participantNamesSQL, contact, participant, AddressProvesSQL(contact, "lower("+participant+".address)"))
}

const participantNamesSQL = `CASE WHEN %[2]s.address IS NULL
		              OR EXISTS (SELECT 1 FROM contact_email se
		                          WHERE se.contact_id = %[2]s.contact_id AND se.email = lower(%[2]s.address)
		                            AND (se.archived_at IS NULL OR EXISTS (
		                                 SELECT 1 FROM contact_email ne
		                                  WHERE ne.email = lower(%[2]s.address) AND ne.archived_at IS NULL
		                                    AND ne.contact_id <> %[2]s.contact_id)))
		            THEN %[2]s.contact_id = %[1]s
		            ELSE %[3]s END`

// ParticipantRowsOfContactSQL is the relation of participant rows that name
// contact by ParticipantNamesSQL, for a reader that starts from the contact.
// The candidates are a union of two indexed arms: rows storing the contact, and
// rows carrying one of their live addresses. A disjunction of the two reads
// every row.
func ParticipantRowsOfContactSQL(contact string) string {
	return fmt.Sprintf(participantRowsOfContactSQL, contact, ParticipantNamesSQL(contact, "pr"))
}

const participantRowsOfContactSQL = `(SELECT pr.* FROM activity_participant pr
		  WHERE pr.id IN (
		        SELECT stored.id FROM activity_participant stored WHERE stored.contact_id = %[1]s
		        UNION ALL
		        SELECT carried.id FROM contact_email ce
		          JOIN activity_participant carried ON lower(carried.address) = ce.email
		         WHERE ce.contact_id = %[1]s AND ce.archived_at IS NULL)
		    AND %[2]s)`

// ParticipantContactsSQL is the subquery of contact ids participant row
// `participant` names by ParticipantNamesSQL. The candidates are the stored
// contact and the live owners of the address.
func ParticipantContactsSQL(participant string) string {
	return fmt.Sprintf(participantContactsSQL, participant, ParticipantNamesSQL("named.id", participant))
}

const participantContactsSQL = `(SELECT named.id FROM (
		    SELECT %[1]s.contact_id
		    UNION SELECT ce.contact_id FROM contact_email ce
		     WHERE ce.email = lower(%[1]s.address) AND ce.archived_at IS NULL) named(id)
		  WHERE named.id IS NOT NULL AND %[2]s)`

// AddressProvesSQL holds when a lowercase address proves who contact is: they
// hold it now and no other contact ever held it. A reassigned address proves
// nothing about who wrote older mail, so every writer that names a party from
// an address after the fact asks this.
func AddressProvesSQL(contact, address string) string {
	return fmt.Sprintf(addressProvesSQL, contact, address)
}

const addressProvesSQL = `(EXISTS (SELECT 1 FROM contact_email own
		         WHERE own.contact_id = %[1]s AND own.archived_at IS NULL AND own.email = %[2]s)
		 AND NOT EXISTS (SELECT 1 FROM contact_email other
		         WHERE other.email = %[2]s AND other.contact_id <> %[1]s))`

// ContactExchangeSQL admits an activity to the contact's last-touch clock: an
// exchange that has happened, and, when inbound, one the contact wrote. A
// colleague's message on a thread the contact only belongs to is not heard
// from them. The reconnect lane and the colleague edge's last_at both read it.
func ContactExchangeSQL(activity, contact, now string) string {
	return InteractionCountsSQL(activity, now) + " AND " + LastActivityCountsSQL(activity, now) +
		" AND (" + activity + ".direction IS DISTINCT FROM 'inbound' OR " + TheyWroteSQL(contact, activity) + ")"
}

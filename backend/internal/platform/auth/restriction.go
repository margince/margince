// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package auth

// A restriction of processing (GDPR Art. 18) is a live processing_restriction
// stop in communication_suppression. These clauses are the one spelling of it
// in SQL; consent writes the stop and storekit asks with them too.

// LiveRestrictionSQL is the predicate on a communication_suppression row alias
// that makes it a restriction in force.
func LiveRestrictionSQL(alias string) string {
	return alias + ".kind = 'processing_restriction' AND " + alias + ".revoked_at IS NULL"
}

// ActivityRestrictedSQL is an EXISTS clause, true when a restricted contact is
// a sender or recipient of the activity activityExpr names, or is linked to
// it. A participant matches by contact id or by an address the contact holds,
// so mail from a restricted contact counts before capture links it.
// activityExpr is a compile-time, table-qualified column reference, never
// request input: an unqualified name would bind to the inner tables.
func ActivityRestrictedSQL(activityExpr string) string {
	return `EXISTS (
		SELECT 1 FROM communication_suppression restriction_stop
		 WHERE ` + LiveRestrictionSQL("restriction_stop") + `
		   AND restriction_stop.contact_id IN (
		       SELECT restriction_party.contact_id FROM activity_participant restriction_party
		        WHERE restriction_party.activity_id = ` + activityExpr + `
		       UNION ALL
		       SELECT restriction_email.contact_id FROM activity_participant restriction_party
		         JOIN contact_email restriction_email ON lower(restriction_email.email) = lower(restriction_party.address)
		        WHERE restriction_party.activity_id = ` + activityExpr + `
		       UNION ALL
		       SELECT restriction_link.contact_id FROM activity_link restriction_link
		        WHERE restriction_link.activity_id = ` + activityExpr + ` AND restriction_link.entity_type = 'contact'))`
}

// ContactRestrictedSQL is an EXISTS clause, true while the contact contactExpr
// names is restricted. contactExpr is a compile-time, table-qualified column
// reference or a placeholder, never request input.
func ContactRestrictedSQL(contactExpr string) string {
	return `EXISTS (SELECT 1 FROM communication_suppression restriction_stop
		 WHERE ` + LiveRestrictionSQL("restriction_stop") + ` AND restriction_stop.contact_id = ` + contactExpr + `)`
}

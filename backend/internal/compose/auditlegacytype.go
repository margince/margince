// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The word audit rows written before the record type was renamed still carry.
//
// Every other table that filed a record type by name was rewritten when the
// rename landed. audit_log was not, and cannot be: `trg_audit_no_mutate`
// refuses an UPDATE on it, which is the property that makes the trail worth
// having. So a row written before the rename says the old word forever, and
// every read of the trail BY RECORD TYPE has to answer for both.
//
// This is the one place the old word survives on purpose. It is a fact about
// history rather than a second spelling of the vocabulary: nothing WRITES it,
// the two readers below only widen a match, and the day no pre-rename row is
// left the constant can go with them.
//
// Both readers matter, and neither fails loudly:
//
//   - humanPrecedence joins the trail to find who last wrote a field. A miss
//     reads as "no human ever touched this", so an agent overwrites a human's
//     value without the staging approval that exists to stop exactly that.
//   - auditExportScope arms a bounded export per record type. A miss drops the
//     row from the export silently, and an export that is quietly short is
//     worse than one that refuses.
const legacyAuditCompanyType = "organization"

// auditCompanyTypes is what an audit read must match to mean "a company":
// today's word and the one the trail already holds.
func auditCompanyTypes() []string {
	return []string{string(recordTypeCompany), legacyAuditCompanyType}
}

// auditTypesFor is auditCompanyTypes for the company and the plain word for
// everything else — no other record type was renamed, so no other trail is
// split across two words.
func auditTypesFor(entityType string) []string {
	if entityType == string(recordTypeCompany) {
		return auditCompanyTypes()
	}
	return []string{entityType}
}

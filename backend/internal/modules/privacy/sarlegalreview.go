// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package privacy

// sarLegalReviewSection exports what legal was asked and answered about the deals
// the subject holds a stake in, in the colleagues' own words. The text is held
// about the deal and erased with the contact, so the export owes it too.
func sarLegalReviewSection(pkg *SARPackage) sarSection {
	return sarSection{&pkg.LegalReviews, `SELECT lr.deal_id, lr.status, lr.created_at AS requested_at,
		          lr.decided_at, lr.note, lr.comments
		   FROM legal_review lr
		   WHERE lr.deal_id IN (` + subjectLiveDealIDsSQL + `)`, nil}
}

// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package privacy

// The Art. 15 chapter covering the confirm links the workspace mailed a subject.
//
// Its own file because sarsections.go reached the 500-line ceiling, and this is
// the chapter that comes off cleanly: one section, one table, and a withholding
// rule that needs its reasons stated at length beside it rather than buried in a
// file about every other chapter.

// sarConsentLinkSections report what became of every confirm link the
// workspace mailed this subject.
//
// A grant on file says the subject agreed. It does not say whether they were
// ever asked, when, or whether the link they were sent was one they actually
// answered — and that is the difference between a completed double opt-in and a
// row somebody typed. The mail itself is already in the Activities section; what
// was missing is the link's own lifecycle, so a subject reading their export
// could see "you hold a marketing consent for me" without seeing the round trip
// it rests on.
//
// TWO COLUMNS ARE WITHHELD, for different reasons:
//
// token_hash, because the row is a live bearer credential that opens this
// subject's own record. Even hashed it is not exported: an Art. 15 package is
// assembled by an admin and handed over through whatever channel they choose,
// and a credential that widens with every copy has no business in it. The
// subject was already sent it, in the mail that carried the link.
//
// delivered_to, because it is the address the workspace CHOSE at issuance, and
// the subject's addresses are already exported in full in their own section.
// Repeating one here would add nothing they do not know while putting a live
// address into a second place in the package.
//
// What remains is timing, outcome and — for a consent link — the purpose it
// asked about, which is the answer Art. 15 asks for and carries no credential at
// all. The purpose join is LEFT because a record-confirmation link names none:
// an inner join would drop every one of them, and a subject asked only to check
// their details would read an empty section as never having been written to.
//
// `outcome` is derived rather than exported as three nullable timestamps,
// because "expired unanswered" is a fact about the subject's own history that a
// reader should not have to compute by comparing dates.
//
// Supersession is tested BEFORE expiry, and that ordering is the point. Issuing
// a fresh link retires the pending one by setting its expires_at to the new
// link's issue time (consent's confirmtoken.go), so a superseded row is
// bit-identical to one the subject let lapse. Reporting it as
// "expired_unanswered" would tell a subject they ignored a link the workspace
// itself withdrew — a statement about their conduct that is not true. The
// matching clauses mirror the writer's: same person, same kind, same purpose,
// which is exactly the scope it supersedes within.
func sarConsentLinkSections(pkg *SARPackage) []sarSection {
	return []sarSection{
		{&pkg.ConsentLinks, `SELECT ct.kind, cp.key AS purpose, ct.issued_at, ct.expires_at,
		          ct.opened_at, ct.consumed_at,
		          CASE WHEN ct.consumed_at IS NOT NULL THEN 'answered'
		               WHEN EXISTS (
		                   SELECT 1 FROM confirm_token later
		                    WHERE later.person_id = ct.person_id
		                      AND later.kind = ct.kind
		                      AND later.purpose_id IS NOT DISTINCT FROM ct.purpose_id
		                      AND later.issued_at > ct.issued_at
		               ) THEN 'superseded_by_a_newer_link'
		               WHEN ct.expires_at <= now() THEN 'expired_unanswered'
		               WHEN ct.opened_at IS NOT NULL THEN 'opened_not_answered'
		               ELSE 'awaiting_answer'
		          END AS outcome
		   FROM confirm_token ct
		   LEFT JOIN consent_purpose cp ON cp.id = ct.purpose_id
		   WHERE ct.person_id = $1`, nil},
	}
}

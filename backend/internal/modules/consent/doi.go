// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package consent

// Why there is no double-opt-in token here any more.
//
// A double opt-in is evidence because the data subject completed it from their
// own mailbox. The issuance endpoint returned the plaintext token to the
// authenticated operator, and RecordConsent accepted that same value back, so
// one person could mint a token, read it off their screen and redeem it — a
// round trip the subject never took part in, recorded as a confirmed one. The
// deliver flag that was supposed to mail it reached an audit payload and no
// mailer, so no installation ever sent one.
//
// Both halves are gone rather than fixed in place: the confirmation a purpose
// needs is now the mailbox proof earned by spending a link that was mailed to
// the person's own live primary address (mailboxproof.go, confirmsubmit.go),
// which is the property the token was standing in for and failing to provide.
//
// consent_doi_token keeps its rows. They are history — issuance and redemption
// that really happened — and Art. 17 erasure and the retention sweep still
// delete them with the subject. Nothing writes the table now.

// purposeIDField names the wire path every purpose refusal on this surface
// points at. Named once because several refusals use it, and a field slot holds
// a wire field path, never prose.
const purposeIDField = "purpose_id"

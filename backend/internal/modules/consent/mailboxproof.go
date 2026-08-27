// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package consent

// MailboxProof records how a caller established that a subject controls the
// address a consent decision was made from.
//
// A double opt-in answers ONE question: does the person submitting an address
// actually control it? A web signup form cannot answer it — anyone may type
// anyone's address into one — so the confirmation mail answers it instead, by
// proving that whoever clicks reads that mailbox.
//
// A surface the subject reached by redeeming a SINGLE-USE link delivered to
// their own address has already answered it, by the same mechanism and before
// the consent question was asked. What the German norm requires is that the
// grant be demonstrable, so the proof travels with it rather than being
// re-enacted: the link's own evidence lands on the proof row.
//
// Single-use is what makes this work, and it is not a detail. The preference
// centre's token is deliberately reusable and lives up to 210 days, so anyone
// who ever holds that URL could replay a grant — including re-granting after a
// withdrawal. A replayable credential shows a mailbox was reached once, never
// that this person chose this now. That is why no preference-link constant
// appears below.
//
// This is deliberately not a bool. "The mailbox was proven" is a claim a reader
// of the consent record must be able to evaluate years later, and a true/false
// leaves them nothing to evaluate.
type MailboxProof string

const (
	// MailboxUnproven is the zero value, and the honest answer for any caller
	// that took the address as input rather than sending something to it.
	MailboxUnproven MailboxProof = ""

	// MailboxProvenByConfirmLink is the confirm-details page: reached only by
	// redeeming a confirm_token, which is hashed at rest, expires in 14 days,
	// is superseded by any fresh issuance, and is spent in the same transaction
	// as the answer it carried.
	MailboxProvenByConfirmLink MailboxProof = "confirm_link"
)

// proves reports whether this proof stands in for a double opt-in round trip.
// Only a named proof does; the zero value never does, so a caller that forgets
// to say how it knows is treated as not knowing.
func (p MailboxProof) proves() bool { return p != MailboxUnproven }

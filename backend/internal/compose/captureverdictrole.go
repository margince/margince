// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"

	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/platform/mailrole"
)

// addressIsARoleMailbox settles a sender by its address, after the mailbox
// owner's own decision and before any model call.
//
// `support@`, `billing@` and a helpdesk vendor's ticket address name a queue
// rather than a person. The correspondence is real — somebody answers, and the
// mail stays visible — but there is no human to name, and the small local model
// this lane runs on answered `person` for exactly these often enough to put
// contacts called "Billing" and "support" in a founder's CRM.
//
// Deterministic on purpose. A question with a right answer that can be read off
// the address should not be spent on a model call whose answer varies, and the
// ledger then settles the address so later mail from the same queue costs
// nothing either.
//
// It sits BELOW the owner's override in judgeOne: a person who tells this
// product that a shared mailbox is a contact they want has answered the
// question, and a rule that overruled them would make the correction temporary.
//
// The vocabulary is platform/mailrole, shared with the tier ladder and with
// people's name parser, so the three doors give one answer for one address.
//
// Held by: TestOnlyOnePackageDeclaresRoleMailboxes (backend/gates/rolemailboxonelist_test.go)
func addressIsARoleMailbox(email string) bool {
	_, role := mailrole.Match(email)
	return role
}

// askAHumanInstead retires a sender no model can judge, so a person decides.
//
// An installation with AI turned off must not simply leave the row where it is.
// Nothing else advances a pending disposition: `unsure` is what the review queue
// reads, only the judging pass writes it, and a row nobody will ever judge looks
// exactly like one whose turn has not come. So the question would stay open
// forever, invisible, and the contacts those senders should have become would
// silently never be created.
//
// Retiring it is the honest answer rather than a fallback: the product cannot
// answer this one, and it says so to the only party who can.
func (e *CounterpartyVerdictEngine) askAHumanInstead(
	ctx context.Context, row capture.PendingCounterparty,
) (int, error) {
	// No measurement: there was no model to ask.
	if err := e.pending.Retire(ctx, row, "no model is configured to judge this sender",
		capture.VerdictMeasurement{}); err != nil {
		return 0, err
	}
	return 1, nil
}

// clearsItsFloor answers whether one model answer is confident enough to stand,
// at the floor its OWN kind has to clear.
//
// A creating answer needs more, because the two mistakes are not the same size.
// Refusing a contact leaves the mail visible and the question answerable by a
// person; creating one puts a record in a shared CRM, and that is the failure
// this lane exists over — a founder found departments, a language teacher and
// his own address filed as business contacts, and a barely-above-floor `person`
// was indistinguishable from a confident one. So `person` and `advisor` need
// verdictCreateFloor and everything else needs verdictConfidenceFloor.
//
// The sibling confidentiality lane is asymmetric for the mirror reason: an
// OPENING answer needs more there, because publishing is the irreversible
// direction. Here creating is.
//
// Below its floor the answer is not refused, it is re-asked once and then made a
// question for a person — an `unsure` sender is escalated rather than dismissed.
func clearsItsFloor(answer verdictResult) bool {
	floor := verdictConfidenceFloor
	if createsARecord(answer.Verdict) {
		floor = verdictCreateFloor
	}
	return float64(answer.Confidence) >= floor
}

// createsARecord reports whether this kind puts a person in the CRM.
//
// It is a second statement of what apply's effect switch does, because that
// switch is control flow and cannot be read as data. A kind added there that
// creates, and not added here, would silently keep the LOWER floor — so a test
// holds the two together rather than a comment claiming they agree.
//
// Held by: TestEveryCreatingKindNeedsTheHigherFloor (backend/internal/compose/captureverdictkinds_test.go)
func createsARecord(kind string) bool {
	return kind == capture.KindPerson || kind == capture.KindAdvisor
}

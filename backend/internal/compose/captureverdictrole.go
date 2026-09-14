// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"fmt"

	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/platform/mailrole"
)

// addressIsARoleMailbox settles a sender by its address, after the mailbox
// owner's own decision and before any model call.
//
// `support@`, `billing@` and a helpdesk vendor's ticket address name a queue
// rather than a contact. The correspondence is real — somebody answers, and the
// mail stays visible — but there is no human to name, and the small local model
// this lane runs on answered `contact` for exactly these often enough to put
// contacts called "Billing" and "support" in a founder's CRM.
//
// Deterministic on purpose. A question with a right answer that can be read off
// the address should not be spent on a model call whose answer varies, and the
// ledger then settles the address so later mail from the same queue costs
// nothing either.
//
// It sits BELOW the owner's override in judgeOne: a contact who tells this
// product that a shared mailbox is a contact they want has answered the
// question, and a rule that overruled them would make the correction temporary.
//
// The vocabulary is platform/mailrole, shared with the tier ladder and with
// contacts's name parser, so the three doors give one answer for one address.
//
// Held by: TestOnlyOnePackageDeclaresRoleMailboxes (backend/gates/rolemailboxonelist_test.go)
func addressIsARoleMailbox(email string) bool {
	_, role := mailrole.Match(email)
	return role
}

// addressNamesNoContact settles a sender whose ADDRESS says nobody is behind it
// — an expense tool's `receipts@`, a billing product's `noreply@`, a bulk relay
// — before any model call.
//
// The sibling above refuses a role mailbox, where the correspondence is real
// and there is simply no human to name. This refuses the other shape: nobody
// answers at all. Both are deterministic for the same reason, and this one was
// missing exactly where it mattered. The tier ladder asks it at T1
// (capture.recordWorthy), but a DEFERRED sender never passes T1 — it goes to a
// model instead, and nothing stood between that answer and a contact record.
//
// What the gap cost, from a ten-year import: `receipts@expensify.com` was asked
// about sixteen times. Fifteen answers were `transactional`; one came back
// `contact` at 0.95 — over the create floor — and minted a contact called
// "Receipts" carrying 340 activities. `noreply@fastbill.com` produced a contact
// named "BERATUNG JUDITH ANDRESEN", the billing product's own customer, because
// the tool sends under its customer's letterhead.
//
// Settled as `transactional` rather than refused silently, so the ledger says
// what the address is and later mail from it costs no model call either.
//
// The vocabulary is capture's, shared with the tier ladder, so both doors give
// one answer for one address.
//
// list is the operator's `transactional_never` allowlist and may be nil.
func addressNamesNoContact(email, domain string, list *capture.TransactionalList) bool {
	return !capture.AddressCouldNameAContact(email, domain, list)
}

// strayVerdictHistory is how many settled non-contact answers an address needs
// before one contradicting `contact` answer stops being allowed to create on
// its own.
//
// Three, not one: a sender genuinely can change — a newsletter address a
// company later staffs with a human, a shop's noreply that becomes a real
// mailbox — and a single earlier `noise` answer is thin evidence against a
// confident new one. Three settled answers saying nobody is there is a pattern,
// and the fourth disagreeing with all of them is the shape that put a contact
// called "Receipts" in a founder's CRM.
//
// The bound is deliberately on the CREATING direction only. A `contact` answer
// creates a record in a shared CRM; every other kind creates nothing, so a
// stray one costs a hidden message the owner can release rather than a contact
// they must find and delete.
const strayVerdictHistory = 3

// strayAgainstItsOwnHistory reports whether this answer creates a contact at an
// address this workspace has repeatedly already concluded has nobody behind it.
//
// A verdict acts on the answer in front of it, so the rare wrong answer wins by
// being last. That is what a ten-year import demonstrated: sixteen questions
// about one expense tool's receipts address, fifteen answered `transactional`,
// one answered `contact` at 0.95 — over the create floor — and the one created
// the record. Nothing re-read the fifteen.
//
// This reads them. It does not overrule the model: it refuses to let a single
// answer act ALONE against a settled history, and routes the disagreement to a
// human instead. The model may still be right — a role mailbox really can gain
// a human — and a human is the one who should say so.
// It reports the count alongside the verdict, so the retirement beside it can
// tell a human what the disagreement actually was.
func (e *CounterpartyVerdictEngine) strayAgainstItsOwnHistory(
	ctx context.Context, row capture.PendingCounterparty, answer verdictResult,
) (bool, int, error) {
	if !createsARecord(answer.Verdict) {
		return false, 0, nil
	}
	settled, err := e.pending.TimesJudgedNotAContact(ctx, row.Email)
	if err != nil {
		return false, 0, err
	}
	return settled >= strayVerdictHistory, settled, nil
}

// askAboutAStrayAnswer retires a creating answer that contradicts the address's
// own settled history, so a human decides rather than the last roll of the dice.
//
// Retired at `unsure` — the same terminal state a below-floor answer reaches —
// because that is what the review queue reads. The measurement travels with it:
// a human asked to settle this is owed what the model actually said, and
// "it answered contact at 0.95 after fifteen transactional answers" is the whole
// question.
func (e *CounterpartyVerdictEngine) askAboutAStrayAnswer(
	ctx context.Context, row capture.PendingCounterparty, answers []verdictResult,
	servedModel string, settled int,
) (int, error) {
	// The reason carries the KIND and the COUNT, because the confidence and the
	// model are all Retire stores and neither says what the disagreement was. A
	// human opening this row is being asked to settle "it answered contact once,
	// after fifteen answers saying nobody is there" — and without both numbers
	// they get the question with none of the evidence.
	kind := ""
	if len(answers) == 1 {
		kind = answers[0].Verdict
	}
	reason := fmt.Sprintf(
		"answered %q against %d settled answers naming no contact at this address", kind, settled)
	if err := e.pending.Retire(ctx, row, reason,
		lastMeasurement(nil, "", answers, servedModel)); err != nil {
		return 0, err
	}
	return 1, nil
}

// askAHumanInstead retires a sender no model can judge, so a human decides.
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
// contact; creating one puts a record in a shared CRM, and that is the failure
// this lane exists over — a founder found departments, a language teacher and
// his own address filed as business contacts, and a barely-above-floor `contact`
// was indistinguishable from a confident one. So `contact` and `advisor` need
// verdictCreateFloor and everything else needs verdictConfidenceFloor.
//
// The sibling confidentiality lane is asymmetric for the mirror reason: an
// OPENING answer needs more there, because publishing is the irreversible
// direction. Here creating is.
//
// Below its floor the answer is not refused, it is re-asked once and then made a
// question for a contact — an `unsure` sender is escalated rather than dismissed.
func clearsItsFloor(answer verdictResult) bool {
	floor := verdictConfidenceFloor
	if createsARecord(answer.Verdict) {
		floor = verdictCreateFloor
	}
	return float64(answer.Confidence) >= floor
}

// createsARecord reports whether this kind puts a contact in the CRM.
//
// It is a second statement of what apply's effect switch does, because that
// switch is control flow and cannot be read as data. A kind added there that
// creates, and not added here, would silently keep the LOWER floor — so a test
// holds the two together rather than a comment claiming they agree.
//
// Held by: TestEveryCreatingKindNeedsTheHigherFloor (backend/internal/compose/captureverdictkinds_test.go)
func createsARecord(kind string) bool {
	return kind == capture.KindContact || kind == capture.KindAdvisor
}

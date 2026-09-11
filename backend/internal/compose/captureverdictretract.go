// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// A noise verdict reaches the RECORD the sender already has, not only their
// mail. The verdict often arrives after the contact: capture creates on
// commit, the ledger is drained later, and a sender judged noise today may
// have been minted a person under an earlier, looser creation rule. Hiding
// their mail while their contact stands leaves "receipts@" on the people list
// forever — the exact junk the verdict said does not belong there.
//
// The two halves stay in their own modules, on the pattern the confidentiality
// engine set: compose asks which records the address still holds, and people
// archives them through its own writer, so the write shape holds and a
// retraction lands an audit row exactly like a human's archive.

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/platform/database"
)

// The two bounds a caller can put on whose records a verdict may withdraw.
//
// retractEveryOwners is for an answer about the ADDRESS: a newsletter is noise
// in every mailbox it reaches, so a colleague's copy of the record goes too.
// retractOwnersOnly is for an answer about one SEAT — the owner's own keep_out,
// and the `personal` kind, which says this mailbox's correspondence with the
// address is private. Neither says anything about a colleague who genuinely
// does business with them, and archiving a colleague's contact on one seat's
// private conversation would be this feature causing the harm it exists to
// prevent.
//
// TestAKeepOutRetractsOnlyTheDecidersOwnContact holds the narrow bound. It
// cannot be held twice over: `uq_person_email_dedupe` makes an address unique
// across live records, so a second seat's copy of one address is a state this
// installation cannot reach — the bound matters for the OTHER shapes a
// colleague's record takes, which is why it is a constant rather than a
// comment.
const (
	retractEveryOwners = false
	retractOwnersOnly  = true
)

// retractSendersContacts withdraws the machine-made records of a sender a
// verdict just covered, on the verdict's own transaction — a row cannot read
// `noise` while the contact it disowns survives a failed second write.
//
// The NOISE caller has already established the sender is not one the workspace
// corresponds with; an address it has provably written to keeps its record
// whatever the classifier called one message. The `personal` caller draws no
// such bound, for the reason its arm gives.
//
// ownersOnly narrows it to one seat, per the constants above.
func (e *CounterpartyVerdictEngine) retractSendersContacts(
	ctx context.Context, tx pgx.Tx, row capture.PendingCounterparty, ownersOnly bool,
) error {
	holders, err := e.people.CaptureOnlyHoldersOfAddressTx(ctx, tx, row.Email)
	if err != nil {
		return err
	}
	for _, h := range holders {
		if ownersOnly && h.OwnerID != row.OwnerID {
			continue
		}
		retracted, err := e.people.RetractCaptureOnlyPersonTx(ctx, tx, h.PersonID, h.OwnerID)
		if err != nil {
			return err
		}
		if retracted {
			e.log.InfoContext(ctx, "counterparty verdict: withdrew the contact a disowned sender had been given",
				"person", h.PersonID.String())
		}
	}
	return nil
}

// retractNoiseJudgedPerTick bounds the sweep below, on the reconcile job's
// usual reasoning: the population shrinks as it is worked, so a small bound
// costs one probe a tick once it is empty.
const retractNoiseJudgedPerTick = 200

// retractNoiseJudgedContacts withdraws the contacts of senders whose noise
// verdict predates the verdict-time retraction above.
//
// A settled sender never re-enters the ledger — that is the promote sweep's
// own rule — so a contact left standing by an already-committed noise verdict
// has no future verdict to catch it. This pass is where those records land.
//
// Correspondence is re-read per contact, inside the same transaction as the
// retraction, because it is the one bound that can CHANGE after the verdict:
// a workspace that has since written to the address has made the sender a
// counterparty, and the old verdict keeps its mail hidden but loses its claim
// on the record.
func (w *linkReconcileWorker) retractNoiseJudgedContacts(ctx context.Context) (int, error) {
	judged, err := w.pending.NoiseJudgedContacts(ctx, retractNoiseJudgedPerTick)
	if err != nil {
		return 0, err
	}
	retracted := 0
	var failed error
	for _, c := range judged {
		// Per contact, each on its own transaction, like every drain in this
		// job: one contact's failure costs that contact, not the sweep.
		if err := database.WithWorkspaceTx(ctx, w.pool, func(tx pgx.Tx) error {
			// The scan committed before this transaction opened, so both the
			// answer that selected the contact and the correspondence bound
			// are re-read here, where the archive can still be called off.
			standing, err := w.pending.NoiseJudgedStandsTx(ctx, tx, c.Email, c.OwnerID)
			if err != nil || !standing.Stands {
				return err
			}
			// Correspondence is the bound that can change after a NOISE
			// answer, and only after one. A `personal` verdict is about whose
			// life the mail belongs to, and a later reply from the owner is
			// what that correspondence looks like rather than evidence
			// against it.
			if standing.Kind != capture.KindPersonal {
				corresponds, err := w.pending.CorrespondsWith(ctx, tx, c.Email)
				if err != nil || corresponds {
					return err
				}
			}
			done, err := w.store.RetractCaptureOnlyPersonTx(ctx, tx, c.PersonID, c.OwnerID)
			if err != nil {
				return err
			}
			if done {
				retracted++
			}
			return nil
		}); err != nil {
			failed = errors.Join(failed, fmt.Errorf("retracting %s: %w", c.PersonID, err))
		}
	}
	return retracted, failed
}

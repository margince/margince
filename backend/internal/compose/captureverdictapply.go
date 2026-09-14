// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// Committing a verdict: who said it, how sure they were, and what it costs to
// be wrong in each direction.

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/platform/database"
)

// apply commits one verdict. The ledger resolution and whatever the verdict
// causes share a transaction, so a row can never read `real` without the records
// it promised, nor `noise` without the hiding it authorized.
//
// Resolve's compare-and-set decides who acts: only the caller that actually
// closed the row runs the effect, which makes a replayed job or a raced sibling
// a no-op rather than a second creation.
func (e *CounterpartyVerdictEngine) apply(
	ctx context.Context, row capture.PendingCounterparty, kind string, ownerSaidSo bool,
	measured capture.VerdictMeasurement,
) (bool, error) {
	verdict, known := statusForKind(kind)
	if !known {
		return false, fmt.Errorf("verdict: %q is not a sender kind", kind)
	}
	var acted bool
	var triageDomain string
	err := database.WithWorkspaceTx(ctx, e.pool, func(tx pgx.Tx) error {
		settled, err := reconcileWithTheOwner(ctx, tx, row, answered{
			kind: kind, verdict: verdict, measured: measured, byOwner: ownerSaidSo,
		})
		if err != nil {
			return err
		}
		kind, verdict, measured, ownerSaidSo = settled.kind, settled.verdict, settled.measured, settled.byOwner

		won, err := e.pending.ResolveAs(ctx, tx, row, verdict, kind, verdictReason, ownerSaidSo, measured)
		if err != nil || !won {
			return err
		}
		acted = true
		// Exhaustive over verdictKinds, held by TestEveryVerdictKindHasAnEffect
		// rather than by this comment: two kinds once reached the prompt with no
		// arm here, and the claim of exhaustiveness is what stopped anybody
		// checking. A `default` that fell through to hideNoise is how a new kind
		// would silently start hiding real mail, so there is none.
		switch kind {
		case capture.KindContact:
			triageDomain, err = e.createContactForVerdict(ctx, tx, row)
			return err
		case capture.KindRoleMailbox, capture.KindCompanySender:
			// Real correspondence with no human to name. The message stays
			// visible; no contact is invented for a mailbox nobody owns.
			return nil
		case capture.KindNewsletter, capture.KindTransactional, capture.KindSpam:
			return e.applyNoise(ctx, tx, row, kind, ownerSaidSo)
		case capture.KindAdvisor:
			// A genuine contact who is the OWNER's. The record is made — a
			// founder's lawyer is somebody they correspond with — and stays
			// owner-scoped, because publishing it to the workspace announces
			// that the founder has a lawyer and what about.
			triageDomain, err = e.createOwnerScopedCounterparty(ctx, tx, row)
			return err
		case capture.KindPersonal:
			// No record, and none kept: a family member is not a counterparty
			// of the business, and one minted before this answer arrived is
			// withdrawn now. The mail itself is not destroyed here — the purge
			// that does that is its own change, with an undo window in front of
			// it — so this withdraws the record and leaves the thread to the
			// mailbox owner.
			//
			// UNBOUNDED BY CORRESPONDENCE, unlike the noise arm above. That
			// bound protects a business counterparty from one misclassified
			// message; here the owner writing back is what a private
			// correspondence LOOKS like, so reading it as evidence of business
			// kept every record this kind is about — a founder's clinic among
			// them.
			//
			// hideNoise is deliberately NOT called. Its scope excludes every
			// address the workspace has written to, which is every address this
			// kind is ever about, so it would be a no-op that read like a hide.
			return e.retractSendersContacts(ctx, tx, row, retractOwnersOnly)
		}
		return fmt.Errorf("verdict: no effect defined for sender kind %q", kind)
	})
	if err != nil {
		// The address is the only identifying detail here and it is already in
		// this workspace's own timeline; the model's answer is not, so the
		// verdict names what was being attempted without echoing content.
		return false, fmt.Errorf("verdict: applying %s to %s: %w", verdict, row.Email, err)
	}
	// Post-commit, like the capture path's own trigger and for the same reason:
	// the records are already durable, and queueing the read that decides their
	// company must not be able to roll them back. A miss is the sweep's.
	if triageDomain != "" && e.triage != nil {
		e.triage.domainPending(ctx, triageDomain)
	}
	return acted, nil
}

// answered is one verdict as it stands: what kind of sender, the ledger status
// that follows, how it was measured, and whether a human is the one saying it.
//
// The four travel together because reconcileWithTheOwner can change all of them
// at once, and a caller that took three of the four would apply a human's
// decision under the model's provenance.
type answered struct {
	kind     string
	verdict  string
	measured capture.VerdictMeasurement
	byOwner  bool
}

// reconcileWithTheOwner re-reads the mailbox owner's own decision on THIS
// transaction and reports the verdict that should actually be applied.
//
// judgeOne reads that decision in a transaction of its own and then spends one
// or two model calls before apply's opens. A contact who answers during that
// gap was answered by a stale read: the contact half already re-checks under
// the contact row lock, but the mail hide and the domain suppression ran on
// what was true before they spoke — so a seat who said "this is business" still
// had the sender's domain suppressed for the whole workspace.
func reconcileWithTheOwner(
	ctx context.Context, tx pgx.Tx, row capture.PendingCounterparty, in answered,
) (answered, error) {
	fresh, err := capture.OverrideForTx(ctx, tx, row.OwnerID, row.Email)
	if err != nil {
		return answered{}, err
	}
	out := in
	if ownerKind, spoke := kindForOverride(fresh); spoke && ownerKind != in.kind {
		// They answered, and differently. THEIR answer is the one applied — the
		// same one judgeOne would reach through ownerDecided on the next pass,
		// taken now rather than leaving the row to wait out a backoff it was
		// already leased under. Abandoning would have been a stall dressed as
		// caution.
		verdict, known := statusForKind(ownerKind)
		if !known {
			return answered{}, fmt.Errorf(
				"verdict: owner decision maps to %q, which is not a sender kind", ownerKind)
		}
		out.kind, out.verdict = ownerKind, verdict
		// The measurement described the MODEL's answer, and this is no longer
		// the model's answer. Recording it against a decision a contact made
		// would put a confidence score on a human.
		out.measured = capture.VerdictMeasurement{}
	}
	// An override that AGREES still makes this the owner's act rather than the
	// model's — the domain suppression turns on that distinction.
	out.byOwner = in.byOwner || fresh != ""
	return out, nil
}

// applyNoise is what a `noise` answer does: it hides the sender's mail, may
// suppress their domain, and withdraws the records capture minted before the
// answer arrived.
//
// Its own method rather than an arm of apply's switch, because it is the only
// arm that DOES anything conditional — the others create a record or create
// nothing — and holding four decisions inside a switch inside a transaction
// callback put the whole function past what a reader can carry at once.
func (e *CounterpartyVerdictEngine) applyNoise(
	ctx context.Context, tx pgx.Tx, row capture.PendingCounterparty, kind string, ownerSaidSo bool,
) error {
	// A seat's own `keep out` does NOT suppress the domain. The two statements
	// are different sizes: the classifier calling a sender noise is a judgement
	// about the sender, and suppressing their domain workspace-wide follows from
	// it; a contact saying "keep this out of my mail" is a statement about their
	// own mailbox, and one rep who once received mail from a partner could
	// otherwise refuse that company to every colleague — with a per-record
	// contact grant and no capture-settings grant at all.
	//
	// The mail hide still runs: it is what "keep out" means, and the noise scope
	// already excludes anything a colleague corresponded with.
	corresponds, err := e.pending.CorrespondsWith(ctx, tx, row.Email)
	if err != nil {
		return err
	}
	if !ownerSaidSo {
		if err := e.suppressSenderDomain(ctx, tx, row, kind, corresponds); err != nil {
			return err
		}
	}
	if err := e.hideNoise(ctx, tx, row); err != nil {
		return err
	}
	// An address the workspace has provably written to keeps its record whatever
	// the classifier called one message — the same bound the domain suppression
	// draws, read once for both.
	if corresponds {
		return nil
	}
	// An owner's own keep_out claims only their record; a machine's noise answer
	// is about the address and reaches every seat's.
	ownersOnly := retractEveryOwners
	if ownerSaidSo {
		ownersOnly = retractOwnersOnly
	}
	return e.retractSendersContacts(ctx, tx, row, ownersOnly)
}

// verdictReason is what the ledger records as the authority for a machine
// disposition, distinguishing it from a T2 registry rule or a human decision.
const verdictReason = "capture_counterparty_verdict"

// hideNoise is the `noise` effect's first stage: the mail stops being visible
// immediately, and its content is redacted later by the sweep (ADR-0072 §4's
// hide-then-redact). The delay is the undo window — the whole reason a verdict
// is allowed to hide anything is that a wrong one can still be taken back.
func (e *CounterpartyVerdictEngine) hideNoise(ctx context.Context, tx pgx.Tx, row capture.PendingCounterparty) error {
	// The scope rule lives with the ledger (noiseMailScope): a verdict may only
	// reach inbound, unattested, unlinked mail from an address the workspace has
	// never written to. Resolved on the SAME transaction, so what is hidden is
	// what was true when the verdict committed.
	due, err := e.pending.NoiseMailForTx(ctx, tx, row.Email, noiseSweepBatch)
	if err != nil {
		return err
	}
	_, err = e.activities.HideCapturedNoiseTx(ctx, tx, due)
	return err
}

// applyOwnerDecision commits what a CONTACT said about a sender.
//
// Split from applyJudged rather than sharing a bool at the call site, so the
// two authorities are visible as two entry points: a reader asking "what can an
// owner's click do" finds one function and the answer beside it.
func (e *CounterpartyVerdictEngine) applyOwnerDecision(ctx context.Context, row capture.PendingCounterparty, kind string) (int, error) {
	// No measurement: a human decided, and no model was asked.
	done, err := e.apply(ctx, row, kind, true, capture.VerdictMeasurement{})
	if err != nil {
		return 0, err
	}
	if done {
		return 1, nil
	}
	return 0, nil
}

// applyJudged commits one above-floor answer and reports whether this caller was
// the one that resolved the row.
//
// measured carries what the model said, so the ledger keeps how sure the answer
// was and which model gave it. A deterministic answer — a role mailbox, read off
// the address — passes the zero value, and the columns stay NULL rather than
// claiming a certainty nobody measured.
func (e *CounterpartyVerdictEngine) applyJudged(
	ctx context.Context, row capture.PendingCounterparty, verdict string, measured capture.VerdictMeasurement,
) (int, error) {
	// The create floor is enforced HERE and not only at the call sites, because
	// this is the chokepoint every model-made answer passes through on its way
	// to a record. A caller that checked the floor and a caller that forgot look
	// identical from the outside, and the one that forgot creates a contact on
	// evidence the product decided was too weak.
	//
	// A deterministic answer carries no measurement and is not subject to a
	// floor: a role mailbox is read off the address, and there is no confidence
	// to be below.
	if measured.Asked && createsARecord(verdict) && measured.Confidence < verdictCreateFloor {
		return 0, fmt.Errorf(
			"verdict: refusing to create a %s record below the create floor", verdict)
	}
	done, err := e.apply(ctx, row, verdict, false, measured)
	if err != nil {
		return 0, err
	}
	if done {
		return 1, nil
	}
	return 0, nil
}

// lastMeasurement is what the model actually said, taken from the re-ask when
// it produced an answer and from the first ask otherwise.
//
// A retirement records the LAST opinion rather than the first, because that is
// the one the floor rejected. Either ask can come back empty — a malformed
// reply is dropped by the validator before it reaches here — so the fallback is
// not decoration: without it a retirement after a failed re-ask would record
// nothing, which reads as "no model was asked" when two were.
func lastMeasurement(
	retry []verdictResult, retryModel string, first []verdictResult, firstModel string,
) capture.VerdictMeasurement {
	if len(retry) == 1 {
		return capture.MeasuredVerdict(float64(retry[0].Confidence), retryModel)
	}
	if len(first) == 1 {
		return capture.MeasuredVerdict(float64(first[0].Confidence), firstModel)
	}
	return capture.VerdictMeasurement{}
}

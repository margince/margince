// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The redemption contract remains; automatic uncertainty offers are withdrawn.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/platform/approvalsubject"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

const (
	// counterpartyProposalKind names the staged offer in the review queue.
	counterpartyProposalKind = approvalsubject.KindCounterparty
	// counterpartyTargetType is what the proposal points at: the captured
	// message, not the sender — the sender has no record yet, which is the
	// whole question being asked.
	counterpartyTargetType = "activity"
	// counterpartyExecutorActor is the principal the accept executes as, in the
	// contract's declared grammar for captured_by.
	counterpartyExecutorActor = contacts.CaptureAcceptActor
)

// counterpartyAcceptEffect builds the approvals.ApprovedEffect for kind
// "capture_counterparty": the human said yes, so the records capture withheld
// are created and the disposition is closed as `real` in the SAME transaction.
//
// There is no matching reject effect, by design. Rejection is the absence of an
// effect — the approvals engine only ever runs the approved branch, which is
// exactly why an offer whose accept merely ADDS records is safe to leave sitting
// in an inbox indefinitely.
func counterpartyAcceptEffect(svc *approvals.Service, store *contacts.Store,
	filer *connectorTagFiler, pending *capture.PendingStore, triage *domainTriageTrigger,
) approvals.ApprovedEffect {
	return func(ctx context.Context, approvalID ids.ApprovalID, proposedChange json.RawMessage, diffHash string) error {
		var proposal approvalsubject.Counterparty
		if err := json.Unmarshal(proposedChange, &proposal); err != nil {
			return fmt.Errorf("compose: decoding the counterparty proposal: %w", err)
		}
		decider, ok := principal.Actor(ctx)
		if !ok {
			return errors.New("compose: counterparty accept without a deciding principal")
		}
		// The records are created by the accept executor on behalf of the human
		// who released it: their approval is on the decision's own audit row,
		// while this write carries the machine provenance that says the records
		// came from a captured message rather than someone typing them in.
		execCtx := principal.WithActor(ctx, principal.Principal{
			Type:       principal.PrincipalSystem,
			ID:         counterpartyExecutorActor,
			UserID:     decider.UserID,
			OnBehalfOf: decider.UserID,
		})
		var triageDomain string
		promotePair, err := contacts.MayPromoteHeldLeads(ctx)
		if err != nil {
			return err
		}
		if err := svc.RedeemAndApply(ctx, approvalID, counterpartyProposalKind, diffHash, func(tx pgx.Tx) error {
			// Asked again as the decider on the write's own transaction: the
			// precheck answered before the decision, and the contact can move.
			if err := store.ProbeCounterpartyMatchTx(ctx, tx, proposal.Email, proposal.Domain); err != nil {
				return err
			}
			var applyErr error
			triageDomain, applyErr = applyCounterpartyAccept(execCtx, tx, store, filer, pending, proposal, promotePair)
			return applyErr
		}); err != nil {
			return err
		}
		// Post-commit, like every other path that opens this question: the
		// records the human just released are durable, and queueing the read
		// that decides their company must not be able to roll them back.
		if triageDomain != "" && triage != nil {
			triage.domainPending(execCtx, triageDomain)
		}
		return nil
	}
}

// applyCounterpartyAccept creates the counterparty and closes its disposition.
// Both on the redemption's transaction, so the ledger can never read `real`
// without the records, nor the records exist under a still-open question.
// It reports the domain still owed a company verdict, for the caller to
// queue once the redemption has committed.
func applyCounterpartyAccept(ctx context.Context, tx pgx.Tx, store *contacts.Store,
	filer *connectorTagFiler, pending *capture.PendingStore, proposal approvalsubject.Counterparty, promotePair bool,
) (string, error) {
	created, err := createCounterpartyRecords(ctx, tx, store, filer, counterpartyCreation{
		DispositionID: proposal.DispositionID,
		Email:         proposal.Email,
		DisplayName:   proposal.DisplayName,
		Domain:        proposal.Domain,
		OwnerID:       proposal.OwnerID,
		ActivityID:    proposal.ActivityID,
		Source:        counterpartyProposalKind,
		CapturedBy:    counterpartyExecutorActor,
	})
	if err != nil {
		return "", err
	}
	// A seat's address keeps `real` for the reason correctRefusedVerdict gives.
	if created.SeatAddress {
		return "", pending.ResolveReviewedAs(ctx, tx, proposal.DispositionID,
			capture.PendingStatusReal, capture.KindContact, seatAddressReason)
	}
	// An address erased while the offer sat in the inbox creates nothing, and
	// the ledger says so rather than reporting `real` for a contact who does not
	// exist — the same correction the machine verdict makes.
	if created.Suppressed {
		return "", pending.ResolveReviewed(ctx, tx, proposal.DispositionID,
			capture.PendingStatusSuppressed, "the address was erased before the review was accepted")
	}
	if promotePair && !created.ContactID.IsZero() {
		if err := store.MergeExistingLeadContactPairForEmailTx(ctx, tx, proposal.Email, created.ContactID); err != nil {
			return "", err
		}
	}
	// Accepting the offer IS the assertion that a contact is behind the address —
	// the queue's whole question is whether to create this contact — so the
	// ledger records that kind rather than leaving the model's guess standing.
	if err := pending.ResolveReviewedAs(ctx, tx, proposal.DispositionID,
		capture.PendingStatusReal, capture.KindContact, "accepted in the review queue"); err != nil {
		return "", err
	}
	// The same release the machine verdict makes, because this door settles the
	// same question. Bounded here too, with the reconciling pass behind it. The
	// widen re-reads the ledger itself, so a resolution that did not take
	// releases nothing.
	if _, err := capture.WidenClearedSenderTx(
		ctx, tx, proposal.Email, clearedSenderLiveBound, activities.RecomputeAudienceTx,
	); err != nil {
		return "", err
	}
	return created.TriageDomain, nil
}

// counterpartyPrecheck refuses an accept whose address already belongs to a
// contact the decider may not file onto. It runs before the decision, so the
// card stays pending; the effect asks again as it writes.
func counterpartyPrecheck(store *contacts.Store) approvals.ReleasePrecheck {
	return func(ctx context.Context, staged, edited json.RawMessage) error {
		var proposal approvalsubject.Counterparty
		if err := json.Unmarshal(staged, &proposal); err != nil {
			return fmt.Errorf("compose: decoding the counterparty proposal: %w", err)
		}
		if err := refuseSenderEdit(proposal, edited); err != nil {
			return err
		}
		return store.ProbeCounterpartyMatch(ctx, proposal.Email, proposal.Domain)
	}
}

// refuseSenderEdit refuses an edit that names another sender. The card asks
// about one address, and an edit aimed at another would file mail elsewhere.
func refuseSenderEdit(staged approvalsubject.Counterparty, edited json.RawMessage) error {
	if len(edited) == 0 {
		return nil
	}
	var after approvalsubject.Counterparty
	if err := json.Unmarshal(edited, &after); err != nil {
		return &approvals.InvalidEditError{Cause: err}
	}
	if !strings.EqualFold(strings.TrimSpace(after.Email), strings.TrimSpace(staged.Email)) ||
		!strings.EqualFold(strings.TrimSpace(after.Domain), strings.TrimSpace(staged.Domain)) {
		return &approvals.InvalidEditError{Cause: errors.New(
			"this card is about one sender; edit the name, not the address or the domain")}
	}
	return nil
}

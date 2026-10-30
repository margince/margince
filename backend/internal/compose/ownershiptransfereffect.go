// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/platform/approvalsubject"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/authz"
)

// errTransferOwnerMoved refuses an accept the record has already left: the
// giver the request names no longer owns it.
var errTransferOwnerMoved = &contacts.TransferRefusal{
	Code:    "owner_changed",
	Message: "This record has changed owner since the request was made, so it cannot be handed over from here. Decline the request.",
}

// transferTargets are the record adapters a transfer locks and moves through:
// the bulk change's, so a handed-over record is written as a reassignment is.
func (t ownershipTransfers) transferTargets() map[string]bulkTarget {
	return map[string]bulkTarget{
		string(crmcontracts.BulkRecordTypeContact): contactBulkTarget{store: t.contacts},
		string(crmcontracts.BulkRecordTypeCompany): companyBulkTarget{store: t.contacts},
	}
}

// transferHandOver is what an accepted transfer moved and what it left.
type transferHandOver struct {
	record string
	deals  deals.RecordDealsHandOver
	tasks  activities.RecordTasks
}

// ownershipTransferAcceptEffect moves the record, and with it the giver's open
// deals and open tasks on it, to the receiver. The move runs as the giver: the
// decider of a request, or the colleague who offered it.
func ownershipTransferAcceptEffect(svc *approvals.Service, t ownershipTransfers) approvals.ApprovedEffect {
	return func(ctx context.Context, approvalID ids.ApprovalID, proposedChange json.RawMessage, diffHash string) error {
		transfer, err := decodeTransfer(proposedChange)
		if err != nil {
			return err
		}
		decider, ok := principal.Actor(ctx)
		if !ok || decider.UserID.IsZero() || decider.UserID != approvalsubject.OwnershipTransferDecider(proposedChange) {
			return fmt.Errorf("a handover is accepted by the colleague it is put to: %w", apperrors.ErrPermissionDenied)
		}
		giverCtx, err := t.asGiver(ctx, transfer)
		if err != nil {
			return err
		}
		return svc.RedeemAndApply(ctx, approvalID, ownershipTransferKind, diffHash, func(tx pgx.Tx) error {
			moved, err := t.handOverTx(giverCtx, tx, transfer)
			if err != nil {
				return err
			}
			return tellAccepted(ctx, tx, approvalID, transfer, moved)
		})
	}
}

// errOfferGiverGone refuses an accept once the colleague who offered the
// record can no longer act: suspended, removed or left.
var errOfferGiverGone = &contacts.TransferRefusal{
	Code:    "owner_unavailable",
	Message: "The colleague who offered this record can no longer hand it over. Decline the offer, or ask an administrator.",
}

// asGiver binds the giver's own authority for the move. In a request the
// giver is the decider already. In an offer the receiver decided, so the
// giver's live grants and seat are read again for the move.
func (t ownershipTransfers) asGiver(ctx context.Context, transfer approvalsubject.OwnershipTransfer) (context.Context, error) {
	if !transfer.Offered() {
		return ctx, nil
	}
	workspace, ok := principal.WorkspaceID(ctx)
	if !ok {
		return nil, errors.New("compose: a handover outside a workspace")
	}
	giver, err := authz.MemberPrincipal(ctx, t.authority, workspace, transfer.FromID)
	if errors.Is(err, apperrors.ErrNotFound) {
		return nil, errOfferGiverGone
	}
	if err != nil {
		return nil, err
	}
	return principal.WithActor(ctx, giver), nil
}

// handOverTx re-reads the owner and the giver's write authority under the
// record's row lock, then moves the record and the giver's open work on it.
func (t ownershipTransfers) handOverTx(ctx context.Context, tx pgx.Tx, transfer approvalsubject.OwnershipTransfer) (transferHandOver, error) {
	target, ok := t.transferTargets()[transfer.RecordType]
	if !ok {
		return transferHandOver{}, fmt.Errorf("compose: a %q cannot be handed over", transfer.RecordType)
	}
	row, err := target.lock(ctx, tx, transfer.RecordID)
	if err != nil {
		return transferHandOver{}, err
	}
	if row.ownerID == nil || *row.ownerID != transfer.FromID {
		return transferHandOver{}, errTransferOwnerMoved
	}
	if err := auth.EnsureWritableLive(ctx, tx, transfer.RecordType, transfer.RecordID); err != nil {
		return transferHandOver{}, err
	}
	if err := auth.EnsureHandOverRecipient(ctx, tx, transfer.ToID); err != nil {
		return transferHandOver{}, err
	}
	consented := auth.WithHandOverConsent(ctx, transfer.ToID)
	from, to := ids.From[ids.UserKind](transfer.FromID), ids.From[ids.UserKind](transfer.ToID)
	reassigner, ok := target.(bulkReassigner)
	if !ok {
		return transferHandOver{}, fmt.Errorf("compose: a %q has no owner to hand on", transfer.RecordType)
	}
	if err := reassigner.reassign(consented, tx, transfer.RecordID, to, row.version); err != nil {
		return transferHandOver{}, err
	}
	out := transferHandOver{record: row.label}
	if out.deals, err = t.deals.HandOverRecordDealsTx(consented, tx, transfer.RecordType, transfer.RecordID, from, to); err != nil {
		return transferHandOver{}, err
	}
	link := activities.RecordLink{EntityType: transfer.RecordType, ID: transfer.RecordID}
	out.tasks, err = t.tasks.HandOverRecordTasksTx(consented, tx, link, from, to)
	return out, err
}

// declineTransferEffect tells the colleague who staged it; the record does not change.
func declineTransferEffect() approvals.DeclinedEffect {
	return func(ctx context.Context, tx pgx.Tx, approvalID ids.ApprovalID, proposedChange json.RawMessage) error {
		transfer, err := decodeTransfer(proposedChange)
		if err != nil {
			return err
		}
		return tellRequester(ctx, tx, approvalID, transfer, transferDeclined)
	}
}

// expireTransferEffect tells the colleague who staged it that nobody answered.
func expireTransferEffect() approvals.ExpiredEffect {
	return func(ctx context.Context, tx pgx.Tx, approvalID ids.ApprovalID, proposedChange json.RawMessage) error {
		transfer, err := decodeTransfer(proposedChange)
		if err != nil {
			return err
		}
		return tellRequester(ctx, tx, approvalID, transfer, transferExpired)
	}
}

// transferPrecheck refuses an accept before it is decided when the record has
// already left the giver, so the card can still be declined.
func (t ownershipTransfers) transferPrecheck() approvals.ReleasePrecheck {
	return func(ctx context.Context, staged, _ json.RawMessage) error {
		transfer, err := decodeTransfer(staged)
		if err != nil {
			return err
		}
		return t.db.Tx(ctx, func(tx pgx.Tx) error {
			owner, _, err := transferRecordTx(ctx, tx, transfer.RecordType, transfer.RecordID)
			if err != nil {
				return err
			}
			if owner == nil || *owner != transfer.FromID {
				return errTransferOwnerMoved
			}
			return nil
		})
	}
}

func decodeTransfer(raw json.RawMessage) (approvalsubject.OwnershipTransfer, error) {
	var transfer approvalsubject.OwnershipTransfer
	if err := json.Unmarshal(raw, &transfer); err != nil {
		return approvalsubject.OwnershipTransfer{}, fmt.Errorf("compose: decoding a handover request: %w", err)
	}
	return transfer, nil
}

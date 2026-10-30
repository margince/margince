// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"encoding/json"
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
)

// errTransferOwnerMoved refuses an accept the record has already left: the
// giver the request names no longer owns it.
var errTransferOwnerMoved = &contacts.TransferRefusal{Code: "owner_changed",
	Message: "This record has changed owner since the request was made, so it cannot be handed over from here. Decline the request."}

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
// deals and open tasks on it, to the receiver. It acts as the giver, who is the
// decider of a request.
func ownershipTransferAcceptEffect(svc *approvals.Service, t ownershipTransfers) approvals.ApprovedEffect {
	return func(ctx context.Context, approvalID ids.ApprovalID, proposedChange json.RawMessage, diffHash string) error {
		transfer, err := decodeTransfer(proposedChange)
		if err != nil {
			return err
		}
		giver, ok := principal.Actor(ctx)
		if !ok || giver.UserID != transfer.FromID {
			return fmt.Errorf("a record is handed over by the colleague who holds it: %w", apperrors.ErrPermissionDenied)
		}
		return svc.RedeemAndApply(ctx, approvalID, ownershipTransferKind, diffHash, func(tx pgx.Tx) error {
			moved, err := t.handOverTx(ctx, tx, transfer)
			if err != nil {
				return err
			}
			return tellAccepted(ctx, tx, approvalID, transfer, moved)
		})
	}
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

// declineTransferEffect tells the colleague who asked; the record does not change.
func declineTransferEffect() approvals.DeclinedEffect {
	return func(ctx context.Context, tx pgx.Tx, approvalID ids.ApprovalID, proposedChange json.RawMessage) error {
		transfer, err := decodeTransfer(proposedChange)
		if err != nil {
			return err
		}
		return tellRequester(ctx, tx, approvalID, transfer, transferDeclined)
	}
}

// expireTransferEffect tells the colleague who asked that nobody answered.
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

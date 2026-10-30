// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"errors"
	"fmt"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/platform/approvalsubject"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/authz"
)

const offerParamTo = "to_id"

// offerReceiverError refuses a colleague who cannot take the record on.
type offerReceiverError struct{ message string }

func (e *offerReceiverError) Error() string { return e.message }

// FieldFault names to_id with the code an owner change answers for an owner
// it will not accept.
func (e *offerReceiverError) FieldFault() (field, code, message string) {
	return offerParamTo, "owner_not_assignable", e.message
}

var (
	errOfferToSelf = &offerReceiverError{message: "You already own this record. Choose the colleague to hand it to."}
	errOfferNoSeat = &offerReceiverError{
		message: "This colleague cannot take the record on: they must be an active colleague who may change records of this kind.",
	}
	errOfferNotOwner = fmt.Errorf("only the record's owner offers it to a colleague; a manager reassigns it directly: %w",
		apperrors.ErrPermissionDenied)
)

// Offer puts the caller's own record in a colleague's inbox. created is false
// when the caller's open offer to that colleague already stood.
func (t ownershipTransfers) Offer(ctx context.Context, recordType string, id, to ids.UUID, note string) (ownershipRequestView, bool, error) {
	if utf8.RuneCountInString(note) > ownershipNoteMaxRunes {
		return ownershipRequestView{}, false, &ownershipNoteError{}
	}
	giver, err := requireOfferer(ctx, recordType, to)
	if err != nil {
		return ownershipRequestView{}, false, err
	}
	if err := t.receiverHolds(ctx, recordType, to); err != nil {
		return ownershipRequestView{}, false, err
	}
	var out ownershipRequestView
	created := false
	err = t.db.Tx(ctx, func(tx pgx.Tx) error {
		if err := holdOwnRecord(ctx, tx, recordType, id, giver); err != nil {
			return err
		}
		if err := auth.EnsureHandOverRecipient(ctx, tx, to); err != nil {
			return asOfferReceiverError(err)
		}
		if out, err = ownRequestTx(ctx, tx, recordType, id, transferOffered); err == nil && out.Transfer.ToID == to {
			return nil
		} else if err != nil && !errors.Is(err, apperrors.ErrNotFound) {
			return err
		}
		if err := refuseBesideOpenTransfer(ctx, tx, recordType, id); err != nil {
			return err
		}
		err := t.stageTx(ctx, tx, approvalsubject.OwnershipTransfer{
			RecordType: recordType, RecordID: id, FromID: giver, ToID: to,
			RequestedBy: giver, DeciderID: to, Note: note,
		})
		if errors.Is(err, approvals.ErrNoDecider) {
			return errOfferNoSeat
		}
		if err != nil {
			return err
		}
		created = true
		out, err = ownRequestTx(ctx, tx, recordType, id, transferOffered)
		return err
	})
	return out, created, err
}

// requireOfferer admits a human signed in as themselves, offering a record
// type a transfer moves to somebody other than themselves.
func requireOfferer(ctx context.Context, recordType string, to ids.UUID) (ids.UUID, error) {
	p, ok := principal.Actor(ctx)
	if !ok || p.Type != principal.PrincipalHuman || p.UserID.IsZero() {
		return ids.Nil, fmt.Errorf("a record is offered by its owner: %w", apperrors.ErrPermissionDenied)
	}
	if _, ok := transferRecords[recordType]; !ok {
		return ids.Nil, &contacts.InvalidRecordTypeError{RecordType: recordType}
	}
	if to == p.UserID {
		return ids.Nil, errOfferToSelf
	}
	return p.UserID, nil
}

// receiverHolds refuses a colleague whose own role cannot change records of
// this type, who could never see the offer to accept it.
func (t ownershipTransfers) receiverHolds(ctx context.Context, recordType string, to ids.UUID) error {
	workspace, ok := principal.WorkspaceID(ctx)
	if !ok {
		return errors.New("compose: an offer outside a workspace")
	}
	member, err := authz.MemberPrincipal(ctx, t.authority, workspace, to)
	if errors.Is(err, apperrors.ErrNotFound) {
		return errOfferNoSeat
	}
	if err != nil {
		return err
	}
	if !member.Permissions.Allows(recordType, principal.ActionRead) || !member.Permissions.Allows(recordType, principal.ActionUpdate) {
		return errOfferNoSeat
	}
	return nil
}

// holdOwnRecord holds the record for the rest of the transaction and refuses
// unless the caller owns it and may change it.
func holdOwnRecord(ctx context.Context, tx pgx.Tx, recordType string, id, giver ids.UUID) error {
	if err := auth.HoldWritableLive(ctx, tx, recordType, id); err != nil {
		return err
	}
	owner, _, err := transferRecordTx(ctx, tx, recordType, id)
	if err != nil {
		return err
	}
	if owner == nil || *owner != giver {
		return errOfferNotOwner
	}
	return nil
}

func asOfferReceiverError(err error) error {
	if _, ineligible := errors.AsType[*auth.AssigneeNotAllowedError](err); ineligible {
		return errOfferNoSeat
	}
	return err
}

// The refusals that name the open transfer in the way, by who meets it.
const (
	offeredAlready   = "This record is already offered to %s. Withdraw that offer first."
	requestedAlready = "%s has asked for this record. Answer that request first."
	offeredToOther   = "The owner has offered this record to %s. You can ask for it once that offer is answered."
)

// refuseBesideOpenTransfer keeps one open transfer per record for an owner
// about to offer it.
func refuseBesideOpenTransfer(ctx context.Context, tx pgx.Tx, recordType string, id ids.UUID) error {
	return refuseOpenTransfer(ctx, tx, recordType, id, func(open approvalsubject.OwnershipTransfer) (ids.UUID, string) {
		if open.Offered() {
			return open.ToID, offeredAlready
		}
		return open.RequestedBy, requestedAlready
	})
}

// refuseBesideOffer refuses a request while the owner's offer of the record
// is open. Colleagues' requests stand side by side for the owner to answer.
func refuseBesideOffer(ctx context.Context, tx pgx.Tx, recordType string, id ids.UUID) error {
	return refuseOpenTransfer(ctx, tx, recordType, id, func(open approvalsubject.OwnershipTransfer) (ids.UUID, string) {
		if open.Offered() {
			return open.ToID, offeredToOther
		}
		return ids.Nil, ""
	})
}

// refuseOpenTransfer answers transfer_pending for the first open transfer on
// the record that blocks names, with the colleague and sentence it gives.
func refuseOpenTransfer(ctx context.Context, tx pgx.Tx, recordType string, id ids.UUID,
	blocks func(approvalsubject.OwnershipTransfer) (ids.UUID, string),
) error {
	open, err := approvals.OpenRequestsForTargetTx(ctx, tx, ownershipTransferKind, recordType, id)
	if err != nil {
		return err
	}
	for _, request := range open {
		transfer, err := decodeTransfer(request.ProposedChange)
		if err != nil {
			return err
		}
		named, message := blocks(transfer)
		if message == "" {
			continue
		}
		name, err := colleagueName(ctx, tx, named)
		if err != nil {
			return err
		}
		return &contacts.TransferRefusal{Code: "transfer_pending", Message: fmt.Sprintf(message, name)}
	}
	return nil
}

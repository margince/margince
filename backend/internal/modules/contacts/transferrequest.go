// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

// Whether a colleague may ask a record's owner to hand it over, or for edit
// access. The ask is for someone who can see a record and may change one of its
// kind. They cannot change this one, because a colleague owns it.

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// transferableRecords are the record types a colleague may ask to be handed,
// each with the statement that reads its owner and its name.
var transferableRecords = map[string]string{
	entityContact: `SELECT owner_id, full_name FROM contact WHERE id = $1 FOR SHARE`,
	entityCompany: `SELECT owner_id, display_name FROM company WHERE id = $1 FOR SHARE`,
}

// TransferRefusal is a record the caller may see but may not ask for. Code is
// the contract's machine code; the status is 409.
type TransferRefusal struct{ Code, Message string }

func (e *TransferRefusal) Error() string { return e.Message }

// MessageFault answers the code and the sentence the caller acts on.
func (e *TransferRefusal) MessageFault() (code, message string) { return e.Code, e.Message }

func (e *TransferRefusal) Unwrap() error { return apperrors.ErrConflict }

var (
	errTransferUnowned = &TransferRefusal{
		Code:    "unowned",
		Message: "Nobody owns this record yet. Take ownership of it instead.",
	}
	errTransferAlreadyYours = &TransferRefusal{
		Code:    "already_yours_to_change",
		Message: "You can already change this record, so there is nothing to ask for.",
	}
)

// TransferableRecord is the record a request asks for, read under a share lock
// so its owner cannot move until the request is filed.
type TransferableRecord struct {
	OwnerID ids.UUID
	Label   string
}

// TransferRequestableTx answers the record's owner when the calling human may
// ask them to hand it over. Only a contact or a company can be handed over.
func TransferRequestableTx(ctx context.Context, tx pgx.Tx, recordType string, id ids.UUID) (TransferableRecord, error) {
	readOwner, ok := transferableRecords[recordType]
	if !ok {
		return TransferableRecord{}, &InvalidRecordTypeError{RecordType: recordType}
	}
	return OwnerAskableTx(ctx, tx, recordType, id, func() (*ids.UUID, string, error) {
		var owner *ids.UUID
		var label string
		err := tx.QueryRow(ctx, readOwner, id).Scan(&owner, &label)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, "", apperrors.ErrNotFound
		}
		return owner, label, err
	})
}

// OwnerAskableTx answers the record's owner when the calling human may ask
// them about a record they see and cannot change: a handover or edit access.
// readOwner reads the owner, nil for nobody, and the record's name.
//
// An unseen record answers 404. A missing update grant or a read seat answers
// 403. An unowned record, or one already theirs to change, answers 409.
func OwnerAskableTx(
	ctx context.Context, tx pgx.Tx, recordType string, id ids.UUID, readOwner func() (*ids.UUID, string, error),
) (TransferableRecord, error) {
	if err := requireOwnerAsker(ctx, recordType); err != nil {
		return TransferableRecord{}, err
	}
	if err := auth.EnsureVisibleLive(ctx, tx, recordType, id); err != nil {
		return TransferableRecord{}, err
	}
	owner, label, err := readOwner()
	if err != nil {
		return TransferableRecord{}, fmt.Errorf("contacts: reading the owner of the record asked about: %w", err)
	}
	if err := auth.RequireRowScopeNarrowerThan(ctx, principal.RowScopeAll, errTransferAlreadyYours); err != nil {
		return TransferableRecord{}, err
	}
	writable, err := auth.WritableLiveBy(ctx, tx, recordType, id)
	if err != nil {
		return TransferableRecord{}, err
	}
	if writable {
		return TransferableRecord{}, errTransferAlreadyYours
	}
	if owner == nil {
		return TransferableRecord{}, errTransferUnowned
	}
	return TransferableRecord{OwnerID: *owner, Label: label}, nil
}

// requireOwnerAsker admits a user or their agent on a seat that can write,
// who holds the grants to read and change this record type.
func requireOwnerAsker(ctx context.Context, recordType string) error {
	p, ok := principal.Actor(ctx)
	if !ok || !p.HumanOrItsAgent() || p.UserID.IsZero() {
		return fmt.Errorf("a record's owner is asked by a colleague: %w", apperrors.ErrPermissionDenied)
	}
	if !p.SeatType.CanMutate() {
		return fmt.Errorf("a read seat does not take on records: %w", apperrors.ErrPermissionDenied)
	}
	if err := auth.Require(ctx, recordType, principal.ActionRead); err != nil {
		return err
	}
	return auth.Require(ctx, recordType, principal.ActionUpdate)
}

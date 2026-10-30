// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

// Whether a colleague may ask a record's owner to hand it over. The ask is for
// someone who can see a contact or company and may change one. They cannot
// change this one, because a colleague owns it.

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
	errTransferUnowned = &TransferRefusal{Code: "unowned",
		Message: "Nobody owns this record yet. Take ownership of it instead."}
	errTransferAlreadyYours = &TransferRefusal{Code: "already_yours_to_change",
		Message: "You can already change this record, so there is nothing to ask for."}
)

// TransferableRecord is the record a request asks for, read under a share lock
// so its owner cannot move until the request is filed.
type TransferableRecord struct {
	OwnerID ids.UUID
	Label   string
}

// TransferRequestableTx answers the record's owner when the calling human may
// ask them to hand it over. A record the caller cannot see answers 404. A
// caller without the update grant or on a read seat answers 403. A record that
// is unowned or already theirs to change answers 409.
func TransferRequestableTx(ctx context.Context, tx pgx.Tx, recordType string, id ids.UUID) (TransferableRecord, error) {
	readOwner, ok := transferableRecords[recordType]
	if !ok {
		return TransferableRecord{}, &InvalidRecordTypeError{RecordType: recordType}
	}
	if err := requireTransferAsker(ctx, recordType); err != nil {
		return TransferableRecord{}, err
	}
	if err := auth.EnsureVisibleLive(ctx, tx, recordType, id); err != nil {
		return TransferableRecord{}, err
	}
	var owner *ids.UUID
	var out TransferableRecord
	err := tx.QueryRow(ctx, readOwner, id).Scan(&owner, &out.Label)
	if errors.Is(err, pgx.ErrNoRows) {
		return TransferableRecord{}, apperrors.ErrNotFound
	}
	if err != nil {
		return TransferableRecord{}, fmt.Errorf("contacts: reading the owner of the record asked for: %w", err)
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
	out.OwnerID = *owner
	return out, nil
}

// requireTransferAsker admits a human on a seat that can write who holds the
// grant to change this record type.
func requireTransferAsker(ctx context.Context, recordType string) error {
	p, ok := principal.Actor(ctx)
	if !ok || p.Type != principal.PrincipalHuman {
		return fmt.Errorf("a handover is asked for by a colleague: %w", apperrors.ErrPermissionDenied)
	}
	if !p.SeatType.CanMutate() {
		return fmt.Errorf("a read seat does not take on records: %w", apperrors.ErrPermissionDenied)
	}
	if err := auth.Require(ctx, recordType, principal.ActionRead); err != nil {
		return err
	}
	return auth.Require(ctx, recordType, principal.ActionUpdate)
}

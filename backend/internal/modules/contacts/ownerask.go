// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

// Whether a colleague may ask a record's owner to hand it over, or for edit
// access. The ask is for someone who can see a record and may change one of its
// kind. They cannot change this one, because a colleague owns it.

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// OwnerAskRefusal is a record the caller may see but may not ask about. Code is
// the contract's machine code; the status is 409.
type OwnerAskRefusal struct{ Code, Message string }

func (e *OwnerAskRefusal) Error() string { return e.Message }

// MessageFault answers the code and the sentence the caller acts on.
func (e *OwnerAskRefusal) MessageFault() (code, message string) { return e.Code, e.Message }

func (e *OwnerAskRefusal) Unwrap() error { return apperrors.ErrConflict }

var (
	errAskUnowned = &OwnerAskRefusal{
		Code:    "unowned",
		Message: "Nobody owns this record yet. Take ownership of it instead.",
	}
	errAskAlreadyYours = &OwnerAskRefusal{
		Code:    "already_yours_to_change",
		Message: "You can already change this record, so there is nothing to ask for.",
	}
)

// AskedRecord is the record a request asks about, read under a share lock
// so its owner cannot move until the request is filed.
type AskedRecord struct {
	OwnerID ids.UUID
	Label   string
}

// OwnerAskableTx answers the record's owner when the calling human may ask
// them about a record they see and cannot change: a handover or edit access.
// readOwner reads the owner, nil for nobody, and the record's name.
//
// An unseen record answers 404. A missing update grant or a read seat answers
// 403. An unowned record, or one already theirs to change, answers 409.
func OwnerAskableTx(
	ctx context.Context, tx pgx.Tx, recordType string, id ids.UUID, readOwner func() (*ids.UUID, string, error),
) (AskedRecord, error) {
	if err := requireOwnerAsker(ctx, recordType); err != nil {
		return AskedRecord{}, err
	}
	if err := auth.EnsureVisibleLive(ctx, tx, recordType, id); err != nil {
		return AskedRecord{}, err
	}
	owner, label, err := readOwner()
	if err != nil {
		return AskedRecord{}, fmt.Errorf("contacts: reading the owner of the record asked about: %w", err)
	}
	if err := auth.RequireRowScopeNarrowerThan(ctx, principal.RowScopeAll, errAskAlreadyYours); err != nil {
		return AskedRecord{}, err
	}
	writable, err := auth.WritableLiveBy(ctx, tx, recordType, id)
	if err != nil {
		return AskedRecord{}, err
	}
	if writable {
		return AskedRecord{}, errAskAlreadyYours
	}
	if owner == nil {
		return AskedRecord{}, errAskUnowned
	}
	return AskedRecord{OwnerID: *owner, Label: label}, nil
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

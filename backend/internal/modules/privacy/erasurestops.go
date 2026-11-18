// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package privacy

// An erasure and a writer of the subject's stops. Every stop writer takes the
// subject's stop locks before any row, and the erasure takes them first too.
// So a stop recorded meanwhile lands wholly before the cascade or after it.

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// erasureKeep answers, under the subject's stop locks, whether something the
// subject recorded keeps them from this erasure.
type erasureKeep func(ctx context.Context, tx pgx.Tx, subject ids.ContactID) error

// admitErasure takes the subject's stop locks, asks keep under them, and then
// refuses a contact the erasure may not touch and resolves its floor.
func (e *Eraser) admitErasure(ctx context.Context, tx pgx.Tx, subject ids.ContactID, keep erasureKeep) (FloorPolicy, error) {
	if err := storekit.LockStopKey(ctx, tx, subject.String()); err != nil {
		return FloorPolicy{}, err
	}
	if err := storekit.LockSubjectsStopAddresses(ctx, tx, subject.UUID); err != nil {
		return FloorPolicy{}, err
	}
	if keep != nil {
		if err := keep(ctx, tx, subject); err != nil {
			return FloorPolicy{}, err
		}
	}
	return e.erasableUnderFloor(ctx, tx, subject)
}

// eraseContactForRetention is the retention policy's contact/erase. A refusal
// inside its floor keeps the contact, as it does in the selector. One recorded
// after the pass selected the contact is seen only here, under the locks.
// An Art. 17 request is not held by that floor: its cascade puts the subject's
// addresses on the erasure list, which refuses every send to them.
func (e *Eraser) eraseContactForRetention(ctx context.Context, contactID ids.UUID) error {
	_, err := e.runContactErasure(ctx, contactID, causeRetention, keptByOptOutFloor)
	return err
}

func keptByOptOutFloor(ctx context.Context, tx pgx.Tx, subject ids.ContactID) error {
	return refuseIfOptOutKeptNow(ctx, tx, "contact_id", subject.UUID)
}

// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package auth

// The whole admission a single-record read or edit passes, as one call per
// verb. The row probes in rowscope.go and writescope.go answer only "which
// rows"; a record page and a "who can see this" answer both need the object
// grant and the seat ceiling as well, and asking the same composition from both
// is what keeps them agreeing.

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// EnsureReadable admits a read of one record: the object grant on its type,
// then the row gate. An archived record still reads — the record page opens one
// through its archived filter — so the live filter is the caller's to add.
func EnsureReadable(ctx context.Context, tx pgx.Tx, table string, id ids.UUID) error {
	if err := Require(ctx, table, principal.ActionRead); err != nil {
		return err
	}
	return EnsureVisible(ctx, tx, table, id)
}

// EnsureChangeable admits an edit of one record: the seat ceiling, the object's
// update grant, then write authority over a LIVE row.
//
// The ceiling is asked here although the session door already refuses a read
// seat's mutating request, because this is also the answer a "who can change
// this" read gives about somebody who is not making a request at all. It
// compares against SeatRead rather than asking CanMutate for the reason
// writeAuthorityPredicateAs gives: an internal principal with no seat to
// resolve is not a read seat.
func EnsureChangeable(ctx context.Context, tx pgx.Tx, table string, id ids.UUID) error {
	p, err := rbacActor(ctx)
	if err != nil {
		return err
	}
	if p.SeatType == principal.SeatRead {
		return fmt.Errorf("%s.%s: %w", table, principal.ActionUpdate, apperrors.ErrSeatTierInsufficient)
	}
	if err := Require(ctx, table, principal.ActionUpdate); err != nil {
		return err
	}
	return EnsureWritableLive(ctx, tx, table, id)
}

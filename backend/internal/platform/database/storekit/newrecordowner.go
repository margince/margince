// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package storekit

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// NewRecordOwner settles the owner_id a manual create stamps: a named owner
// once it has been checked, else OwnerOrActor's default.
//
// Naming somebody on a create is an assignment, so it asks the same question
// an ownership change asks (auth.EnsureNewRecordOwner) rather than trusting
// the FK, which proves only that the row exists — it admits a suspended seat,
// an agent, and a colleague outside the caller's own write scope. The fallback
// needs no check: the actor is the seat making the call.
//
// It takes a transaction because the question is a query, which is why a
// create asks it in its transactional body rather than beside its parse.
func NewRecordOwner(ctx context.Context, tx pgx.Tx, owner *ids.UserID) (*ids.UserID, error) {
	if owner == nil {
		return OwnerOrActor(ctx), nil
	}
	if err := auth.EnsureNewRecordOwner(ctx, tx, owner.UUID); err != nil {
		return nil, err
	}
	return owner, nil
}

// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The edge from a module asking "may THAT seat read this object" to identity,
// which owns the role tables that answer it.
//
// Almost every authorization question in the tree is about the caller, and
// platform/auth answers those without help. This one is about somebody else: an
// operator nominating a colleague to receive lead escalations, where the seat
// being named is not the seat asking. The grants live in identity's tables and
// a module never imports a sibling, so compose carries the edge.

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/modules/people"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// seatReadsLeads answers whether one seat's grants admit reading a lead.
//
// The caller's own authority is NOT this seam's question: the surface offering
// the nomination gates that, and this reads the nominee's grants and nothing
// else. It runs inside the caller's transaction so the answer and the write it
// gates commit together.
func seatReadsLeads(_ *pgxpool.Pool) people.SeatReadsLeads {
	return func(ctx context.Context, tx pgx.Tx, seat ids.UUID) (bool, error) {
		return identity.SeatAllows(ctx, tx, ids.From[ids.UserKind](seat), "lead", principal.ActionRead)
	}
}

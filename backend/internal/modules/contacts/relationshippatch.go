// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

// What a patch to a relationship must clear before it lands.
//
// Its own file beside relationship.go, which owns the read, the lock and the
// statement. The refusals are a different subject from the mechanics: they are
// asked together, about one row, and a reader deciding whether a patch is
// allowed should meet them as a list rather than interleaved with the
// transaction that carries them out.

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// refusePatch runs the refusals a patch clears once the row is read and locked:
// what this edge is, who may change it, what the patched value may say, and
// whether the caller is looking at the version they think they are.
//
// Its own function because those are asked TOGETHER and about the same row — a
// reader answering "may this patch land" should not have to assemble the answer
// from four places in a transaction that also locks, updates and reads back.
func refusePatch(ctx context.Context, tx pgx.Tx, current relationshipRow, in UpdateRelationshipInput) error {
	// A kind is only checked on CREATE, and this path names an existing row
	// — so the exclusion has to be re-stated here or the generic surface
	// becomes the side door the vocabulary closed.
	if err := refuseGenericProjectCompany(current.Kind); err != nil {
		return err
	}
	// Same rule as create: editing an edge is editing its anchor, so it
	// takes both halves of the anchor's authority — the object grant, and
	// the row.
	anchorObject, _ := relationshipAnchor(current.Kind)
	if err := auth.Require(ctx, anchorObject, principal.ActionUpdate); err != nil {
		return err
	}
	if err := ensureRelationshipAnchorWritable(ctx, tx, current.Kind, current.endpoints()); err != nil {
		return err
	}
	// A patch cannot move an edge's endpoints or its kind, but it CAN move
	// the role — and for a billing contact the role is what the row means.
	// Without this, the generic patch surface is the way to turn a stated
	// capacity into a word the create path refuses. Asked on the PATCHED
	// value: a patch that leaves role alone keeps whatever already passed.
	if in.Role != nil {
		if err := validBillingContactRole(current.Kind, in.Role); err != nil {
			return err
		}
	}
	if in.IfVersion != nil && *in.IfVersion != current.Version {
		return apperrors.ErrVersionSkew
	}
	return nil
}

// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// admitPairDecision answers the context a dismissal or its undo writes under.
// Ordinary: write authority over both ends. Under shared merge reviews, a live
// full-seat owner of one end of a contact pair may decide it too. The context
// then admits the two ends through the case merge's own admission.
func (s *Store) admitPairDecision(ctx context.Context, tx pgx.Tx, row DedupeCandidateRow) (context.Context, error) {
	refused := ensurePairWritable(ctx, tx, row.EntityType, row.LeftID, row.RightID)
	if refused == nil || row.EntityType != entityContact || !errors.Is(refused, apperrors.ErrPermissionDenied) {
		return ctx, refused
	}
	policy, err := s.sharedMergeReviewsForDecision(ctx, tx)
	if err != nil {
		return ctx, err
	}
	if !policy {
		return ctx, refused
	}
	ends := []ids.UUID{row.LeftID, row.RightID}
	members, err := readCaseMembers(ctx, tx, ends)
	if err != nil {
		return ctx, err
	}
	pair := contactCase{Members: members}
	actor, ok := principal.Actor(ctx)
	if !ok || len(members) != len(ends) || !pair.actorOwnsAMember(actor, ends) {
		return ctx, refused
	}
	admitted, err := auth.WithContactCaseAdmission(ctx, ends, pair.liveOwners(ends))
	if err != nil {
		return ctx, err
	}
	return admitted, ensurePairWritable(admitted, tx, row.EntityType, row.LeftID, row.RightID)
}

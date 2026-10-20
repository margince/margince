// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"context"
	"slices"

	"github.com/jackc/pgx/v5"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

type RelinkParticipantChange struct {
	ID            ids.UUID
	ContactID     ids.UUID
	Role          string
	UserID        *ids.UUID
	Address       *string
	ChannelUserID *string
	Move          bool
}

func participantRelinkQuery(ctx context.Context, id ids.ActivityID, target ids.UUID, displaced []ids.UUID) (string, []any, int, error) {
	var args []any
	arg := func(v any) int { args = append(args, v); return len(args) }
	idPos, targetPos, displacedPos := arg(id), arg(target), arg(displaced)
	visible, err := auth.ScopeClauseFor(ctx, linkEntityContact, "op", arg)
	if err != nil {
		return "", nil, 0, err
	}
	if visible == "" {
		visible = "true"
	}
	query := storekit.SQLf(`WITH scoped AS (
  SELECT ap.id, ap.activity_id, ap.contact_id, ap.role, ap.user_id, ap.address, ap.channel_user_id,
   row_number() OVER (
    PARTITION BY ap.role, ap.user_id, ap.address, ap.channel_user_id
    ORDER BY ap.id) AS rank
  FROM activity_participant ap
  WHERE ap.activity_id = $%d AND ap.contact_id = ANY($%d::uuid[])
   AND ap.contact_id <> $%d
   AND EXISTS (SELECT 1 FROM contact op WHERE op.id = ap.contact_id AND (%s))
 ) SELECT s.id, s.contact_id, s.role, s.user_id, s.address, s.channel_user_id,
  s.rank = 1 AND NOT EXISTS (
   SELECT 1 FROM activity_participant other
   WHERE other.activity_id = s.activity_id AND other.role = s.role
    AND other.contact_id = $%d
    AND other.user_id IS NOT DISTINCT FROM s.user_id
    AND other.address IS NOT DISTINCT FROM s.address
    AND other.channel_user_id IS NOT DISTINCT FROM s.channel_user_id) AS move
 FROM scoped s`, idPos, displacedPos, targetPos, visible, targetPos)
	return query, args, targetPos, nil
}

func previewRelinkParticipants(ctx context.Context, tx pgx.Tx, id ids.ActivityID, target ids.UUID, displaced []ids.UUID) ([]RelinkParticipantChange, error) {
	query, args, _, err := participantRelinkQuery(ctx, id, target, displaced)
	if err != nil {
		return nil, err
	}
	rows, err := tx.Query(ctx, query+" ORDER BY s.id", args...)
	if err != nil {
		return nil, err
	}
	changes, err := pgx.CollectRows(rows, pgx.RowToStructByPos[RelinkParticipantChange])
	if err != nil {
		return nil, err
	}
	if err := EnsureParticipantRosterVisibleTx(ctx, tx, id, slices.ContainsFunc(changes, func(change RelinkParticipantChange) bool { return change.Role == roleBcc })); err != nil {
		return nil, err
	}
	return changes, nil
}

func EnsureParticipantRosterVisibleTx(ctx context.Context, tx pgx.Tx, id ids.ActivityID, hasBCC bool) error {
	if err := auth.Require(ctx, "activity", principal.ActionRead); err != nil {
		return err
	}
	if err := auth.EnsureActivityContentVisible(ctx, tx, id.UUID); err != nil {
		return err
	}
	if !hasBCC {
		return nil
	}
	sender, err := callerIsSenderSeat(ctx, tx, id)
	if err != nil {
		return err
	}
	if !sender {
		return apperrors.ErrNotFound
	}
	return nil
}

// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// SentFromHereBySeatTx proves mailbox ownership before the provider captures its sent copy.
// Agent stamps carry no durable human attribution, so they cannot prove this ownership.
func SentFromHereBySeatTx(ctx context.Context, tx pgx.Tx, seat ids.UUID, candidates []ids.ActivityID) ([]ids.ActivityID, error) {
	if err := auth.Require(ctx, "activity", principal.ActionRead); err != nil {
		if errors.Is(err, apperrors.ErrPermissionDenied) {
			return []ids.ActivityID{}, nil
		}
		return nil, err
	}
	if seat == ids.Nil || arrivingHuman(ctx) != seat.String() || len(candidates) == 0 {
		return nil, nil
	}
	args := []any{}
	arg := func(value any) int { args = append(args, value); return len(args) }
	content, err := auth.ActivityContentClause(ctx, "a", arg)
	if err != nil {
		return nil, err
	}
	where := fmt.Sprintf("a.id = ANY($%d::uuid[]) AND a.source_system = $%d", arg(candidates), arg(connector.EmailSourceSystem))
	rows, err := tx.Query(ctx, `SELECT a.id, a.captured_by FROM activity a
 WHERE a.archived_at IS NULL AND a.direction = 'outbound' AND a.source = 'manual'
 AND `+where+` AND `+content, args...)
	if err != nil {
		return nil, fmt.Errorf("activities: reading sent thread neighbours: %w", err)
	}
	defer rows.Close()
	var sent []ids.ActivityID
	for rows.Next() {
		var id ids.ActivityID
		var stamp string
		if err := rows.Scan(&id, &stamp); err != nil {
			return nil, fmt.Errorf("activities: reading sent ownership: %w", err)
		}
		if actingHumanOf(stamp) == seat.String() {
			sent = append(sent, id)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("activities: reading sent neighbours: %w", err)
	}
	return sent, nil
}

// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// RenewCommitmentTaskTx updates only open automatic work that no human has adopted or changed.
func (s *Store) RenewCommitmentTaskTx(ctx context.Context, tx pgx.Tx, locator, summary, body string, due time.Time, privateTo *ids.UUID) (bool, error) {
	if err := auth.Require(ctx, "activity", principal.ActionUpdate); err != nil {
		return false, err
	}
	actor, ok := principal.Actor(ctx)
	if !ok || actor.Type != principal.PrincipalSystem {
		return false, nil
	}
	var id ids.UUID
	var assignee *ids.UUID
	var closed bool
	var args []any
	arg := func(value any) int { args = append(args, value); return len(args) }
	err := tx.QueryRow(ctx, fmt.Sprintf(`SELECT a.id, a.archived_at IS NOT NULL OR a.is_done, a.assignee_id
 FROM activity a WHERE a.source_system = $%d AND a.source_id = $%d FOR UPDATE OF a`, arg(CommitmentTaskSource), arg(locator)),
		args...).Scan(&id, &closed, &assignee)
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && closed) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("activities: locking a renewed commitment: %w", err)
	}
	if err := auth.EnsureActivityContentVisibleLive(ctx, tx, id); err != nil {
		return false, err
	}
	allowed, err := renewalAudienceFits(ctx, tx, id, assignee, privateTo)
	if err != nil || !allowed {
		return false, err
	}

	touched, err := storekit.HumanActedSince(ctx, tx, "activity", id, time.Time{})
	if err != nil || touched {
		return false, err
	}
	_, err = updateActivityInTx(ctx, tx, ids.From[ids.ActivityKind](id), UpdateActivityInput{
		Subject: &summary, Body: &body, DueAt: &due,
	})
	return err == nil, err
}

func renewalAudienceFits(ctx context.Context, tx pgx.Tx, id ids.UUID, assignee, privateTo *ids.UUID) (bool, error) {
	if privateTo == nil {
		return true, nil
	}
	if assignee == nil || *assignee != *privateTo {
		return false, nil
	}
	return auth.ActivityAudienceLimitedToTx(ctx, tx, id, *privateTo)
}

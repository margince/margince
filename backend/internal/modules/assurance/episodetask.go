// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package assurance

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// ReviewTypes are the finding types delivered as one task per episode. Their
// task is kept once delivered: completing it does not resolve the review, and a
// second task for the same unchanged episode would ask the same thing again.
func ReviewTypes() []string { return []string{TypeDealViability} }

// EpisodeTask reads a finding's episode and the task that episode was
// delivered as, if any. Held FOR UPDATE, so two deliveries of one episode
// serialize here and the second finds the first one's link.
func (s *Store) EpisodeTask(ctx context.Context, tx pgx.Tx, exceptionID ids.UUID) (int, *ids.UUID, error) {
	if err := auth.Require(ctx, rbacBundle, principal.ActionUpdate); err != nil {
		return 0, nil, err
	}
	var episode int
	var task *ids.UUID
	err := tx.QueryRow(ctx, `
		SELECT episode, task_activity_id FROM assurance_exception
		 WHERE id = $1
		 FOR UPDATE`, exceptionID).Scan(&episode, &task)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, nil, apperrors.ErrNotFound
	}
	if err != nil {
		return 0, nil, fmt.Errorf("assurance: reading the finding's episode: %w", err)
	}
	return episode, task, nil
}

// LinkEpisodeTask records the task an episode was delivered as. It refuses to
// move an existing link: once delivered, the episode keeps its task.
func (s *Store) LinkEpisodeTask(ctx context.Context, tx pgx.Tx, exceptionID ids.UUID, episode int, task ids.UUID) error {
	if err := auth.Require(ctx, rbacBundle, principal.ActionUpdate); err != nil {
		return err
	}
	tag, err := tx.Exec(ctx, `
		UPDATE assurance_exception
		SET task_activity_id = $3, updated_at = now()
		WHERE id = $1 AND episode = $2 AND task_activity_id IS NULL`, exceptionID, episode, task)
	if err != nil {
		return fmt.Errorf("assurance: linking the episode's task: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return apperrors.ErrVersionSkew
	}
	_, err = storekit.AuditEvent(ctx, tx, "update", "assurance_exception", exceptionID,
		map[string]any{"episode": episode, "task_activity_id": task.String()})
	return err
}

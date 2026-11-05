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
// task is kept once delivered, because completing it does not resolve the
// review. A second task for the same episode would ask the same thing again.
func ReviewTypes() []string { return []string{TypeDealViability} }

// ReviewEpisode is a review finding's current episode and how far its
// delivery got.
type ReviewEpisode struct {
	Episode int
	// Task is the task the episode was delivered as, nil until it is.
	Task *ids.UUID
	// DeliveredAssignee is the seat the system gave the task. Nil when the task
	// went to the unassigned queue, or somebody else held it at delivery.
	DeliveredAssignee *ids.UUID
}

// Delivery states of a review episode. A finding that is not a review, and an
// episode no pass has queued yet, have none.
const (
	DeliveryPending   = "pending"
	DeliveryFailed    = "failed"
	DeliveryDelivered = "delivered"
)

// EpisodeTask reads a finding's episode and its delivery. Held FOR UPDATE, so
// two deliveries of one episode serialize here and the second finds the first
// one's link.
func (s *Store) EpisodeTask(ctx context.Context, tx pgx.Tx, exceptionID ids.UUID) (ReviewEpisode, error) {
	if err := auth.Require(ctx, rbacBundle, principal.ActionUpdate); err != nil {
		return ReviewEpisode{}, err
	}
	var out ReviewEpisode
	err := tx.QueryRow(ctx, `
		SELECT episode, task_activity_id, delivered_assignee_id FROM assurance_exception
		 WHERE id = $1
		 FOR UPDATE`, exceptionID).Scan(&out.Episode, &out.Task, &out.DeliveredAssignee)
	if errors.Is(err, pgx.ErrNoRows) {
		return ReviewEpisode{}, apperrors.ErrNotFound
	}
	if err != nil {
		return ReviewEpisode{}, fmt.Errorf("assurance: reading the finding's episode: %w", err)
	}
	return out, nil
}

// LinkEpisodeTask records the task an episode was delivered as, and the seat
// the system gave it. It refuses to move an existing link: once delivered, the
// episode keeps its task.
func (s *Store) LinkEpisodeTask(
	ctx context.Context, tx pgx.Tx, exceptionID ids.UUID, episode int, task ids.UUID, assignee *ids.UUID,
) error {
	if err := auth.Require(ctx, rbacBundle, principal.ActionUpdate); err != nil {
		return err
	}
	tag, err := tx.Exec(ctx, `
		UPDATE assurance_exception
		SET task_activity_id = $3, delivered_assignee_id = $4, delivery_state = $5, updated_at = now()
		WHERE id = $1 AND episode = $2 AND task_activity_id IS NULL`,
		exceptionID, episode, task, assignee, DeliveryDelivered)
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

// FollowedOwner records that the episode's task moved with the deal to a new
// owner, so the next owner change is measured against this one.
func (s *Store) FollowedOwner(ctx context.Context, tx pgx.Tx, exceptionID ids.UUID, episode int, assignee ids.UUID) error {
	return s.recordDelivery(ctx, tx, exceptionID, episode, `
		UPDATE assurance_exception SET delivered_assignee_id = $3, updated_at = now()
		 WHERE id = $1 AND episode = $2`, assignee.String(),
		map[string]any{"episode": episode, "delivered_assignee_id": assignee.String()})
}

// QueueDelivery marks an undelivered episode pending. A failed one is pending
// again: the pass that queues it is its retry.
func (s *Store) QueueDelivery(ctx context.Context, tx pgx.Tx, exceptionID ids.UUID, episode int) error {
	return s.recordDelivery(ctx, tx, exceptionID, episode, `
		UPDATE assurance_exception SET delivery_state = $3, updated_at = now()
		 WHERE id = $1 AND episode = $2 AND task_activity_id IS NULL`, DeliveryPending,
		map[string]any{"episode": episode, "delivery_state": DeliveryPending})
}

// RecordDeliveryAttempt counts one attempt at delivering the episode, and marks
// it failed when it was the last one the job gets and left no task.
func (s *Store) RecordDeliveryAttempt(ctx context.Context, tx pgx.Tx, exceptionID ids.UUID, episode int, last bool) error {
	state := DeliveryPending
	if last {
		state = DeliveryFailed
	}
	return s.recordDelivery(ctx, tx, exceptionID, episode, `
		UPDATE assurance_exception
		   SET delivery_attempts = delivery_attempts + 1, updated_at = now(),
		       delivery_state = CASE WHEN task_activity_id IS NULL THEN $3 ELSE delivery_state END
		 WHERE id = $1 AND episode = $2`, state,
		map[string]any{"episode": episode, "delivery_attempt": true, "last_attempt": last})
}

// recordDelivery runs one write of an episode's delivery and audits it. A write
// that matched no row is an episode that ended since: nothing changed, so
// nothing is audited.
func (s *Store) recordDelivery(
	ctx context.Context, tx pgx.Tx, exceptionID ids.UUID, episode int, update, value string, after map[string]any,
) error {
	if err := auth.Require(ctx, rbacBundle, principal.ActionUpdate); err != nil {
		return err
	}
	tag, err := tx.Exec(ctx, update, exceptionID, episode, value)
	if err != nil {
		return fmt.Errorf("assurance: recording the review's delivery: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return nil
	}
	_, err = storekit.AuditEvent(ctx, tx, "update", "assurance_exception", exceptionID, after)
	return err
}

// DeliveryCounts are the open reviews by how far their delivery got. A review
// whose task somebody completed is delivered: the task reached them.
type DeliveryCounts struct {
	Pending, Failed, Delivered int
}

// reviewDeliveryCounts counts the open reviews by delivery.
func reviewDeliveryCounts(ctx context.Context, tx pgx.Tx) (DeliveryCounts, error) {
	var out DeliveryCounts
	err := tx.QueryRow(ctx, `
		SELECT count(*) FILTER (WHERE task_activity_id IS NULL AND delivery_state IS DISTINCT FROM $2),
		       count(*) FILTER (WHERE task_activity_id IS NULL AND delivery_state = $2),
		       count(*) FILTER (WHERE task_activity_id IS NOT NULL)
		  FROM assurance_exception
		 WHERE status = 'open' AND type = ANY($1)`, ReviewTypes(), DeliveryFailed).
		Scan(&out.Pending, &out.Failed, &out.Delivered)
	if err != nil {
		return DeliveryCounts{}, fmt.Errorf("assurance: counting the reviews' deliveries: %w", err)
	}
	return out, nil
}

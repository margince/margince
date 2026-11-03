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
	id, err := s.MutableCommitmentTaskTx(ctx, tx, locator, privateTo)
	if err != nil || id.IsZero() {
		return false, err
	}

	_, err = updateActivityInTx(ctx, tx, ids.From[ids.ActivityKind](id), UpdateActivityInput{
		Subject: &summary, Body: &body, DueAt: &due,
	})
	return err == nil, err
}

// renewalAudienceFits admits a task from private mail only while its holder may
// still hold that work. Nobody but the holder and the mailbox owner may read it.
// A holder whose sent copy was turned back to received mail is refused.
func renewalAudienceFits(ctx context.Context, tx pgx.Tx, id ids.UUID, assignee, privateTo *ids.UUID) (bool, error) {
	if privateTo == nil {
		return true, nil
	}
	if assignee == nil {
		return false, nil
	}
	var source *ids.UUID
	if err := tx.QueryRow(ctx, `SELECT source_activity_id FROM activity WHERE id = $1`, id).Scan(&source); err != nil {
		return false, fmt.Errorf("activities: reading the source of a renewed commitment: %w", err)
	}
	if source == nil {
		return false, nil
	}
	may, err := auth.SeatMayHoldPrivateWorkTx(ctx, tx, *source, privateTo, *assignee)
	if err != nil || !may {
		return false, err
	}
	return auth.ActivityAudienceLimitedToTx(ctx, tx, id, *assignee, *privateTo)
}

// MutableCommitmentTaskTx locks only live automatic work that no human has adopted or changed.
func (s *Store) MutableCommitmentTaskTx(ctx context.Context, tx pgx.Tx, locator string, privateTo *ids.UUID) (ids.UUID, error) {
	if err := auth.Require(ctx, "activity", principal.ActionRead); err != nil {
		return ids.Nil, err
	}
	actor, ok := principal.Actor(ctx)
	if !ok || actor.Type != principal.PrincipalSystem {
		return ids.Nil, nil
	}
	target, err := s.OpenCommitmentTaskTx(ctx, tx, locator, privateTo)
	if err != nil || target.ID.IsZero() {
		return ids.Nil, err
	}
	id := target.ID

	touched, err := storekit.HumanActedSince(ctx, tx, "activity", id, time.Time{})
	if err != nil || touched {
		return ids.Nil, err
	}
	return id, nil
}

// CommitmentTaskTarget preserves the current assignee when only recipients change.
type CommitmentTaskTarget struct {
	ID       ids.UUID
	Assignee *ids.UUID
}

// OpenCommitmentTaskTx locks the target so a closed task cannot acquire recipients.
func (s *Store) OpenCommitmentTaskTx(ctx context.Context, tx pgx.Tx, locator string, privateTo *ids.UUID) (CommitmentTaskTarget, error) {
	if err := auth.Require(ctx, "activity", principal.ActionRead); err != nil {
		return CommitmentTaskTarget{}, err
	}
	var id ids.UUID
	var assignee *ids.UUID
	var closed bool
	var args []any
	arg := func(value any) int { args = append(args, value); return len(args) }
	query := fmt.Sprintf(`SELECT a.id, a.archived_at IS NOT NULL OR a.is_done, a.assignee_id
 FROM activity a WHERE a.source_system = $%d AND a.source_id = $%d FOR UPDATE OF a`, arg(CommitmentTaskSource), arg(locator))
	err := tx.QueryRow(ctx, query, args...).Scan(&id, &closed, &assignee)
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && closed) {
		return CommitmentTaskTarget{}, nil
	}
	if err != nil {
		return CommitmentTaskTarget{}, fmt.Errorf("activities: locking a renewed commitment: %w", err)
	}
	if err := auth.EnsureActivityContentVisibleLive(ctx, tx, id); err != nil {
		return CommitmentTaskTarget{}, err
	}
	allowed, err := renewalAudienceFits(ctx, tx, id, assignee, privateTo)
	if err != nil || !allowed {
		return CommitmentTaskTarget{}, err
	}

	return CommitmentTaskTarget{ID: id, Assignee: assignee}, nil
}

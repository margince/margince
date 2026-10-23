// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/datatask"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// guessedMailboxSeatsDataTask takes off the mailbox owners the participant
// backfill once guessed for mail stamped with a bare `connector:<provider>`:
// the provider's only connection was read as the owner. Such a participant row
// let that seat read the message. Nothing else on the message changes.
func guessedMailboxSeatsDataTask() datatask.Task {
	return datatask.Task{
		ID: "core/1792788759_guessed_mailbox_owners", Version: 1,
		Title: "Removing mailbox owners guessed for older mail", Requires: []string{"core/1788151532"},
		Steps: []datatask.Step{{
			Name: "retire_guessed_mailbox_owners", Limit: 200, MinLimit: 1,
			Audit:    datatask.AuditPolicy{PerRecord: true},
			Estimate: estimateGuessedMailboxSeats, Batch: retireGuessedMailboxSeatsBatch,
		}},
	}
}

func retireGuessedMailboxSeatsBatch(ctx context.Context, tx pgx.Tx, in datatask.BatchIn) (datatask.BatchOut, error) {
	if err := auth.RequireSystem(ctx); err != nil {
		return datatask.BatchOut{}, err
	}
	after, err := datatask.UUIDCursor(in.Cursor)
	if err != nil {
		return datatask.BatchOut{}, err
	}
	found, err := activities.GuessedMailboxActivitiesTx(ctx, tx, after, in.Limit)
	if err != nil {
		return datatask.BatchOut{}, err
	}
	changed := 0
	for _, activityID := range found {
		retired, err := activities.RetireGuessedMailboxSeatsTx(ctx, tx, activityID, in.RunID)
		if err != nil {
			return datatask.BatchOut{}, err
		}
		if retired {
			changed++
		}
	}
	return datatask.AdvanceUUID(in, found, changed)
}

func estimateGuessedMailboxSeats(ctx context.Context, q datatask.Querier, maximum int64) (int64, bool, error) {
	if err := auth.Require(ctx, "data_task", principal.ActionRead); err != nil {
		return 0, false, err
	}
	count, err := activities.CountGuessedMailboxActivities(ctx, q, maximum)
	return min(count, maximum), count > maximum, err
}

// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/datatask"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// auditFieldMailboxSeats names the seats a repair took off a message as its
// mailbox owner, in the audit images.
const auditFieldMailboxSeats = "mailbox_seats"

// guessedMailboxSeatSQL holds for participant p of activity a when p is a seat
// row that grants nothing (auth.ParticipantStandsSQL) and has nothing else to
// stand on: the seat's own mailbox did not deliver the message. That is the row
// the participant backfill once wrote for the provider's only connection. A row
// under a hold is never touched.
func guessedMailboxSeatSQL() string {
	return `a.restricted_at IS NULL AND p.user_id IS NOT NULL AND NOT (` + auth.ParticipantStandsSQL("p", "a") + `)
	   AND NOT EXISTS (SELECT 1 FROM capture_import ci WHERE ci.activity_id = a.id AND ci.user_id = p.user_id)`
}

// GuessedMailboxActivitiesTx locks the next limit activities after the cursor,
// in id order, that carry a guessed mailbox owner.
func GuessedMailboxActivitiesTx(ctx context.Context, tx pgx.Tx, after ids.UUID, limit int) ([]ids.UUID, error) {
	if err := auth.RequireSystem(ctx); err != nil {
		return nil, err
	}
	rows, err := tx.Query(ctx, `SELECT a.id FROM activity a WHERE a.id > $1
		   AND EXISTS (SELECT 1 FROM activity_participant p WHERE p.activity_id = a.id AND `+guessedMailboxSeatSQL()+`)
		 ORDER BY a.id LIMIT $2 FOR UPDATE OF a`, after, limit)
	if err != nil {
		return nil, fmt.Errorf("activities: reading mail with a guessed mailbox owner: %w", err)
	}
	found, err := pgx.CollectRows(rows, pgx.RowTo[ids.UUID])
	if err != nil {
		return nil, fmt.Errorf("activities: reading mail with a guessed mailbox owner: %w", err)
	}
	return found, nil
}

// CountGuessedMailboxActivities counts the activities carrying a guessed
// mailbox owner, reading at most maximum+1. It is the repair's preview.
func CountGuessedMailboxActivities(ctx context.Context, q datatask.Querier, maximum int64) (int64, error) {
	var count int64
	if err := q.QueryRow(ctx, `SELECT count(*) FROM (SELECT 1 FROM activity a
		   WHERE EXISTS (SELECT 1 FROM activity_participant p WHERE p.activity_id = a.id AND `+guessedMailboxSeatSQL()+`)
		 LIMIT $1) guessed`, maximum+1).Scan(&count); err != nil {
		return 0, fmt.Errorf("activities: counting mail with a guessed mailbox owner: %w", err)
	}
	return count, nil
}

// RetireGuessedMailboxSeatsTx takes the guessed mailbox owners off one
// activity, so the message stays unowned. The message, its stored original and
// its other participants stay. The rule is re-read under the row lock, so a
// replay changes nothing.
func RetireGuessedMailboxSeatsTx(ctx context.Context, tx pgx.Tx, activityID, runID ids.UUID) (bool, error) {
	if err := auth.RequireSystem(ctx); err != nil {
		return false, err
	}
	if _, err := storekit.LockRow(ctx, tx, "activity", activityID, storekit.IncludeArchived); err != nil {
		return false, err
	}
	rows, err := tx.Query(ctx, `DELETE FROM activity_participant p USING activity a
		 WHERE a.id = $1 AND p.activity_id = a.id AND `+guessedMailboxSeatSQL()+`
		 RETURNING p.user_id`, activityID)
	if err != nil {
		return false, fmt.Errorf("activities: removing the guessed mailbox owner of %s: %w", activityID, err)
	}
	seats, err := pgx.CollectRows(rows, pgx.RowTo[ids.UUID])
	if err != nil {
		return false, fmt.Errorf("activities: removing the guessed mailbox owner of %s: %w", activityID, err)
	}
	if len(seats) == 0 {
		return false, nil
	}
	auditID, err := storekit.AuditWithEvidence(ctx, tx, "update", "activity", activityID,
		map[string]any{auditFieldMailboxSeats: seats}, map[string]any{auditFieldMailboxSeats: []ids.UUID{}},
		map[string]any{"data_task_run": runID.String()})
	if err != nil {
		return false, err
	}
	// The interaction graph refolds an activity's participants on this event.
	if err := storekit.EmitEvent(ctx, tx, auditID, activityID, crmcontracts.PublicEventActivityUpdated{}); err != nil {
		return false, err
	}
	return true, nil
}

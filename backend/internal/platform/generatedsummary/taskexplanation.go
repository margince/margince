// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package generatedsummary

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// ReadTaskExplanation is the reader's stored explanation of one task.
func ReadTaskExplanation(ctx context.Context, pool *pgxpool.Pool, reader ids.UserID, task ids.ActivityID) (Entry, bool, error) {
	var entry Entry
	found := true
	err := database.WithTx(ctx, pool, func(tx pgx.Tx) error {
		err := tx.QueryRow(ctx, `
			SELECT fingerprint, generated_at, generated_by, payload
			  FROM task_explanation WHERE user_id = $1 AND activity_id = $2`, reader, task).Scan(
			&entry.Fingerprint, &entry.GeneratedAt, &entry.GeneratedBy, &entry.Payload)
		if errors.Is(err, pgx.ErrNoRows) {
			found = false
			return nil
		}
		return err
	})
	if err != nil {
		return Entry{}, false, fmt.Errorf("read the task explanation: %w", err)
	}
	return entry, found, nil
}

// SaveTaskExplanationTx replaces the reader's explanation of one task.
func SaveTaskExplanationTx(ctx context.Context, tx pgx.Tx, reader ids.UserID, task ids.ActivityID, written Entry) error {
	if _, err := tx.Exec(ctx, `
		INSERT INTO task_explanation (user_id, activity_id, fingerprint, generated_at, generated_by, payload)
		VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (user_id, activity_id) DO UPDATE
		SET fingerprint = EXCLUDED.fingerprint, generated_at = EXCLUDED.generated_at,
		    generated_by = EXCLUDED.generated_by, payload = EXCLUDED.payload`,
		reader, task, written.Fingerprint, written.GeneratedAt, written.GeneratedBy, written.Payload); err != nil {
		return fmt.Errorf("store the task explanation: %w", err)
	}
	return nil
}

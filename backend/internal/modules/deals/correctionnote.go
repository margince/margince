// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package deals

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// linkCorrectionNote names the History note that reports corrections, in the
// transaction that wrote both. The note then offers the Undo its corrections
// have. Every writer of a note that reports a Margince change stamps it here.
func linkCorrectionNote(ctx context.Context, tx pgx.Tx, corrections []ids.UUID, note ids.UUID) error {
	if len(corrections) == 0 {
		return nil
	}
	if _, err := tx.Exec(ctx,
		`UPDATE deal_correction SET note_activity_id = $1 WHERE id = ANY($2)`, note, corrections); err != nil {
		return fmt.Errorf("deals: linking the note that reports a change: %w", err)
	}
	return nil
}

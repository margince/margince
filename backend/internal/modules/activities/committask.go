// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/provenance"
)

// CommitmentTaskSource marks a task written for a commitment one of our users
// made in a captured conversation.
const CommitmentTaskSource = provenance.CommitmentTaskSource

// CommitmentTaskWritten reports whether a commitment's task was ever written,
// archived and completed ones included. An archived task is how a rep says
// "not mine to do", so the answer must outlive the archive or the next reading
// of the same conversation would write it again.
func (s *Store) CommitmentTaskWritten(ctx context.Context, tx pgx.Tx, locator string) (bool, error) {
	if err := auth.Require(ctx, "activity", principal.ActionRead); err != nil {
		return false, err
	}
	var written bool
	if err := tx.QueryRow(ctx, `
		SELECT EXISTS (SELECT 1 FROM activity
		                WHERE source_system = $1 AND source_id = $2)`,
		CommitmentTaskSource, locator).Scan(&written); err != nil {
		return false, fmt.Errorf("activities: reading whether the commitment's task exists: %w", err)
	}
	return written, nil
}

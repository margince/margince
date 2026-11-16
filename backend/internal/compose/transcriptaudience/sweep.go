// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package transcriptaudience

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// sweepBatch bounds one workspace's pass; a pass that stops early leaves the
// rest for the next one.
const sweepBatch = 200

// Sweep gives each meeting transcript the readers its meeting has now. A guest
// added to or taken off the meeting, by any writer, reaches the transcript here.
type Sweep struct {
	Pool *pgxpool.Pool
}

// Workspace runs one workspace's pass, as the system.
func (s Sweep) Workspace(ctx context.Context, workspace ids.UUID) error {
	ctx = principal.SystemActing(principal.WithWorkspaceID(ctx, workspace), "system:transcript-audience")
	return database.WithWorkspaceTx(ctx, s.Pool, func(tx pgx.Tx) error {
		_, err := activities.FollowDriftedTranscriptsTx(ctx, tx, sweepBatch)
		return err
	})
}

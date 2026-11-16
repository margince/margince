// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package meetingprofile

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// sweepBatch bounds one workspace's pass; a pass that stops early leaves the
// rest for the next one.
const sweepBatch = 200

// Sweep removes the company fields a meeting filled once its transcript may no
// longer fill them: narrowed to fewer readers, restricted, archived or redacted.
type Sweep struct {
	Pool *pgxpool.Pool
}

// Workspace runs one workspace's pass, as the system.
func (s Sweep) Workspace(ctx context.Context, workspace ids.UUID) error {
	ctx = principal.SystemActing(principal.WithWorkspaceID(ctx, workspace), "system:meeting-profile")
	return database.WithWorkspaceTx(ctx, s.Pool, func(tx pgx.Tx) error {
		stale, err := contacts.StaleMeetingTranscriptsTx(ctx, tx, sweepBatch)
		if err != nil {
			return err
		}
		for _, transcript := range stale {
			if err := contacts.ApplyMeetingProfileTx(ctx, tx, transcript, nil); err != nil {
				return err
			}
		}
		return nil
	})
}

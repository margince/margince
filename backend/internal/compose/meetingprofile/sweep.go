// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package meetingprofile

import (
	"cmp"
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// sweepBatch bounds one transaction of a workspace's pass.
const sweepBatch = 200

// Sweep removes the company fields a meeting filled once its transcript may no
// longer fill them: narrowed to fewer readers, restricted, archived or redacted.
type Sweep struct {
	Pool *pgxpool.Pool
	// Batch is the rows one transaction takes. Zero is sweepBatch.
	Batch int
}

// Workspace runs one workspace's pass, as the system. Each batch deletes what
// it removes, so the pass repeats until a batch comes back short. A field
// another writer holds is left to the next pass.
func (s Sweep) Workspace(ctx context.Context, workspace ids.UUID) error {
	ctx = principal.SystemActing(principal.WithWorkspaceID(ctx, workspace), "system:meeting-profile")
	for {
		var removed int
		if err := database.WithTx(ctx, s.Pool, func(tx pgx.Tx) error {
			var err error
			removed, err = contacts.RetractStaleMeetingProfilesTx(ctx, tx, s.batch())
			return err
		}); err != nil {
			return err
		}
		if removed < s.batch() {
			return nil
		}
	}
}

func (s Sweep) batch() int { return cmp.Or(s.Batch, sweepBatch) }

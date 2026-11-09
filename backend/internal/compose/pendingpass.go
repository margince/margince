// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func runPendingPass(ctx context.Context, pool *pgxpool.Pool, pass func(context.Context) (bool, error)) error {
	more := false
	err := runPerWorkspace(ctx, pool, func(ctx context.Context, workspace ids.UUID) error {
		next, err := pass(principal.WithWorkspaceID(ctx, workspace))
		more = more || next
		return err
	})
	if err != nil {
		return err
	}
	if more {
		return river.JobSnooze(0)
	}
	return nil
}

func pendingWorkOpts(kind string) *river.InsertOpts {
	opts := oneOffPassOpts(kind)
	opts.UniqueOpts = river.UniqueOpts{ByArgs: true, ByState: activeSweepStates}
	return opts
}

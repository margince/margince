// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package recordsources is the data task that moves deals, offers and projects
// whose source predates the fixed list of channels onto a channel.
package recordsources

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/modules/projects"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/datatask"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/systemtext"
)

// correct moves one page of a table's rows, and count says how many are left.
type (
	correct func(ctx context.Context, tx pgx.Tx, after ids.UUID, limit int, runID ids.UUID) ([]ids.UUID, error)
	count   func(ctx context.Context, q datatask.Querier, maximum int64) (int64, error)
)

// DataTask corrects one table per step. The audit row of each corrected record
// keeps the source it held, so the system and id an old value packed stay readable.
func DataTask() datatask.Task {
	return datatask.Task{
		ID: "core/1795363335_record_source_channels", Version: 1,
		Title:    systemtext.Literal("datatask.title.core.1795363335_record_source_channels"),
		Requires: []string{"core/1795363335"},
		Steps: []datatask.Step{
			step("deal_sources", deals.CorrectDealSourcesTx, deals.CountUnlistedDealSources),
			step("offer_sources", deals.CorrectOfferSourcesTx, deals.CountUnlistedOfferSources),
			step("project_sources", projects.CorrectProjectSourcesTx, projects.CountUnlistedProjectSources),
		},
	}
}

func step(name string, move correct, left count) datatask.Step {
	return datatask.Step{
		Name: name, Limit: 500, MinLimit: 1,
		Audit:         datatask.AuditPolicy{PerRecord: true},
		EmptyWhenDone: true,
		Estimate: func(ctx context.Context, q datatask.Querier, maximum int64) (int64, bool, error) {
			if err := auth.Require(ctx, "data_task", principal.ActionRead); err != nil {
				return 0, false, err
			}
			unlisted, err := left(ctx, q, maximum)
			return min(unlisted, maximum), unlisted > maximum, err
		},
		Batch: func(ctx context.Context, tx pgx.Tx, in datatask.BatchIn) (datatask.BatchOut, error) {
			after, err := datatask.UUIDCursor(in.Cursor)
			if err != nil {
				return datatask.BatchOut{}, err
			}
			moved, err := move(ctx, tx, after, in.Limit, in.RunID)
			if err != nil {
				return datatask.BatchOut{}, err
			}
			return datatask.AdvanceUUID(in, moved, len(moved))
		},
	}
}

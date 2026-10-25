// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/datatask"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// seatEmploymentDataTask archives the employment edges capture or a provider
// inferred for a contact that holds a proven seat address. A colleague's
// employer is the workspace; an edge a human made or edited stays.
func seatEmploymentDataTask() datatask.Task {
	return datatask.Task{
		ID: "core/1792894995_seat_employment_edges", Version: 1,
		Title: "Removing employers inferred for colleagues", Requires: []string{"core/1792887436"},
		Steps: []datatask.Step{{
			Name: "retire_seat_employment_edges", Limit: 200, MinLimit: 1,
			Audit:    datatask.AuditPolicy{PerRecord: true},
			Estimate: estimateSeatEmploymentEdges, Batch: retireSeatEmploymentEdgesBatch,
		}},
	}
}

func retireSeatEmploymentEdgesBatch(ctx context.Context, tx pgx.Tx, in datatask.BatchIn) (datatask.BatchOut, error) {
	if err := auth.RequireSystem(ctx); err != nil {
		return datatask.BatchOut{}, err
	}
	after, err := datatask.UUIDCursor(in.Cursor)
	if err != nil {
		return datatask.BatchOut{}, err
	}
	page, err := contacts.RetireSeatEmploymentEdgesTx(ctx, tx, after, in.Limit, in.RunID.String())
	if err != nil {
		return datatask.BatchOut{}, err
	}
	return datatask.AdvanceUUID(in, page.Scanned, page.Retired)
}

func estimateSeatEmploymentEdges(ctx context.Context, q datatask.Querier, maximum int64) (int64, bool, error) {
	if err := auth.Require(ctx, "data_task", principal.ActionRead); err != nil {
		return 0, false, err
	}
	count, err := contacts.CountLiveEmploymentEdges(ctx, q, maximum)
	return min(count, maximum), count > maximum, err
}

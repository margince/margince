// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/datatask"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// leadScoresFollowOriginDataTask repairs what the move from source to origin
// left behind. The first step fills an origin from the capture a lead was
// written from, where the wording resolves to a catalog key. The second scores
// every open lead again. That lifts the penalty the old import weight put on
// leads from a file or a connector. Both steps write through the contacts
// module's transaction functions, and a second run changes nothing.
func leadScoresFollowOriginDataTask(now func() time.Time) datatask.Task {
	return datatask.Task{
		ID: "core/1792811003_lead_scores_follow_origin", Version: 1,
		Title: "Scoring leads by where they came from", Requires: []string{"core/1792801001"},
		Steps: []datatask.Step{
			{
				Name: "origin_from_retained_capture", Limit: 200, MinLimit: 10,
				Audit:    datatask.AuditPolicy{PerRecord: true},
				Estimate: estimateLeadOriginBackfill,
				Batch: func(ctx context.Context, tx pgx.Tx, in datatask.BatchIn) (datatask.BatchOut, error) {
					after, err := datatask.UUIDCursor(in.Cursor)
					if err != nil {
						return datatask.BatchOut{}, err
					}
					scanned, changed, err := contacts.BackfillLeadOriginTx(ctx, tx, after, in.Limit)
					if err != nil {
						return datatask.BatchOut{}, err
					}
					return datatask.AdvanceUUID(in, scanned, changed)
				},
			},
			{
				Name: "rescore_open_leads", Limit: 100, MinLimit: 10,
				Audit:    datatask.AuditPolicy{PerRecord: true},
				Estimate: estimateOpenLeads,
				Batch: func(ctx context.Context, tx pgx.Tx, in datatask.BatchIn) (datatask.BatchOut, error) {
					after, err := datatask.UUIDCursor(in.Cursor)
					if err != nil {
						return datatask.BatchOut{}, err
					}
					scanned, changed, err := contacts.RescoreOpenLeadsTx(ctx, tx, after, in.Limit, now().UTC())
					if err != nil {
						return datatask.BatchOut{}, err
					}
					return datatask.AdvanceUUID(in, scanned, changed)
				},
			},
		},
	}
}

func estimateLeadOriginBackfill(ctx context.Context, q datatask.Querier, maximum int64) (int64, bool, error) {
	if err := auth.Require(ctx, "data_task", principal.ActionRead); err != nil {
		return 0, false, err
	}
	count, err := contacts.CountLeadOriginBackfills(ctx, q, maximum)
	return min(count, maximum), count > maximum, err
}

func estimateOpenLeads(ctx context.Context, q datatask.Querier, maximum int64) (int64, bool, error) {
	if err := auth.Require(ctx, "data_task", principal.ActionRead); err != nil {
		return 0, false, err
	}
	count, err := contacts.CountOpenLeads(ctx, q, maximum)
	return min(count, maximum), count > maximum, err
}

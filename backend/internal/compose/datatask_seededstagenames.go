// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/datatask"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/systemtext"
)

// seededStageNamesDataTask gives the stages an older workspace was seeded with
// their German and Vietnamese names, so a reader in those languages sees no
// English stage. A stage an admin renamed or translated is left alone.
func seededStageNamesDataTask() datatask.Task {
	return datatask.Task{
		ID: "core/1795583688_seeded_stages_translated", Version: 1,
		Title:    systemtext.Literal("datatask.title.core.1795583688_seeded_stages_translated"),
		Requires: []string{"core/1794481502"},
		Steps: []datatask.Step{{
			Name: "translate_seeded_stages", Limit: 200, MinLimit: 10,
			Audit: datatask.AuditPolicy{PerRecord: true},
			Estimate: func(ctx context.Context, q datatask.Querier, maximum int64) (int64, bool, error) {
				if err := auth.Require(ctx, "data_task", principal.ActionRead); err != nil {
					return 0, false, err
				}
				count, err := deals.CountUntranslatedSeededStages(ctx, q, maximum)
				return min(count, maximum), count > maximum, err
			},
			Batch: func(ctx context.Context, tx pgx.Tx, in datatask.BatchIn) (datatask.BatchOut, error) {
				after, err := datatask.UUIDCursor(in.Cursor)
				if err != nil {
					return datatask.BatchOut{}, err
				}
				scanned, changed, err := deals.TranslateSeededStagesTx(ctx, tx, after, in.Limit)
				if err != nil {
					return datatask.BatchOut{}, err
				}
				return datatask.AdvanceUUID(in, scanned, changed)
			},
		}},
	}
}

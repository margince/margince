// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/platform/datatask"
)

// mailDealsDataTask files mail captured before the deal rule under the one
// deal the rule finds for it, from the deals module's candidates. It only
// adds links and asks nobody; mail already on a deal is never touched.
func mailDealsDataTask(pool *pgxpool.Pool) datatask.Task {
	candidates := deals.NewStore(InstallationDB(pool), DealsInstallation())
	return datatask.Task{
		ID: "core/1792680000_captured_mail_filed_on_its_deal", Version: 1,
		Title: "Filing captured mail on the one deal it is about", Requires: []string{"core/1792679518"},
		Steps: []datatask.Step{{
			Name: "file_captured_mail_on_its_deal", Limit: 200, MinLimit: 10,
			Audit:    datatask.AuditPolicy{PerRecord: true},
			Estimate: capture.EstimateDealRepair,
			Batch: func(ctx context.Context, tx pgx.Tx, in datatask.BatchIn) (datatask.BatchOut, error) {
				return capture.RepairDealLinksTx(ctx, tx, in, candidates)
			},
		}},
	}
}

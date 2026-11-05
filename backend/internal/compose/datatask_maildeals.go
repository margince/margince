// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/platform/datatask"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// mailDealsDataTask files mail captured before the deal rule under the one
// deal the rule finds for it, and re-checks the links the rule itself wrote.
// It decides as the mailbox that captured each mail, and asks nobody. It never
// touches a colleague's filing; the operator preview reports what it leaves.
func mailDealsDataTask(pool *pgxpool.Pool) datatask.Task {
	db := InstallationDB(pool)
	repair := mailDealRepair(pool)
	return datatask.Task{
		ID: "core/1792680000_captured_mail_filed_on_its_deal", Version: 1,
		Title: "Filing captured mail on the one deal it is about", Requires: []string{"core/1792679518"},
		Steps: []datatask.Step{{
			Name: "file_captured_mail_on_its_deal", Limit: 200, MinLimit: 10,
			Audit:    datatask.AuditPolicy{PerRecord: true},
			Estimate: capture.EstimateDealRepair,
			Batch: func(ctx context.Context, tx pgx.Tx, in datatask.BatchIn) (datatask.BatchOut, error) {
				// The capturing seat's authority is resolved per workspace,
				// which the executor's system context does not carry.
				if _, bound := principal.WorkspaceID(ctx); !bound {
					ws, err := db.Workspace(ctx)
					if err != nil {
						return datatask.BatchOut{}, err
					}
					ctx = principal.WithWorkspaceID(ctx, ws.UUID)
				}
				return repair.BatchTx(ctx, tx, in)
			},
		}},
	}
}

// mailDealRepair is the repair the data task runs and the operator previews.
func mailDealRepair(pool *pgxpool.Pool) capture.DealRepair {
	return capture.DealRepair{
		Candidates: deals.NewStore(InstallationDB(pool), DealsInstallation()),
		Authority:  identity.NewService(pool), Filer: ActivityDealFiler{},
	}
}

// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/datatask"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// deliveryReportsAfter is the core migration this task waits for.
const deliveryReportsAfter = "core/1792799323"

// deliveryReportsDataTask archives the delivery reports an import filed as
// received mail before the write door refused them. It then re-derives the lead
// status, score and first-response stamp they caused from the remaining
// touches. The sender rule is the one the write door uses.
func deliveryReportsDataTask(pool *pgxpool.Pool, now func() time.Time) datatask.Task {
	return datatask.Task{
		ID: "core/1792823163_imported_delivery_reports", Version: 1,
		Title: "Archiving delivery reports filed as received mail", Requires: []string{deliveryReportsAfter},
		Steps: []datatask.Step{{
			Name: "imported_received_mail", Limit: 200, MinLimit: 10,
			Audit:    datatask.AuditPolicy{PerRecord: true},
			Estimate: estimateImportedReceivedMail,
			Batch: func(ctx context.Context, tx pgx.Tx, in datatask.BatchIn) (datatask.BatchOut, error) {
				return archiveDeliveryReportsBatch(ctx, tx, in, activities.NewStore(InstallationDB(pool)), now())
			},
		}},
	}
}

func archiveDeliveryReportsBatch(
	ctx context.Context, tx pgx.Tx, in datatask.BatchIn, store *activities.Store, asOf time.Time,
) (datatask.BatchOut, error) {
	if err := auth.RequireSystem(ctx); err != nil {
		return datatask.BatchOut{}, err
	}
	after, err := datatask.UUIDCursor(in.Cursor)
	if err != nil {
		return datatask.BatchOut{}, err
	}
	scanned, reports, err := activities.DeliveryReportsTx(ctx, tx, after, in.Limit)
	if err != nil {
		return datatask.BatchOut{}, err
	}
	leads := map[ids.LeadID]bool{}
	for _, report := range reports {
		if err := store.ArchiveDeliveryReportTx(ctx, tx, report.ID); err != nil {
			return datatask.BatchOut{}, err
		}
		for _, lead := range report.Leads {
			leads[lead] = true
		}
	}
	for lead := range leads {
		if err := contacts.ReDeriveLeadFromRemainingTouchesTx(ctx, tx, lead, asOf); err != nil {
			return datatask.BatchOut{}, err
		}
	}
	return datatask.AdvanceUUID(in, scanned, len(reports))
}

func estimateImportedReceivedMail(ctx context.Context, q datatask.Querier, maximum int64) (int64, bool, error) {
	if err := auth.Require(ctx, "data_task", principal.ActionRead); err != nil {
		return 0, false, err
	}
	count, err := activities.CountDeliveryReports(ctx, q, maximum)
	return min(count, maximum), count > maximum, err
}

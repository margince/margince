// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/finance"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/datatask"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/systemtext"
)

// customerLinksDataTask names the customer link on every mirrored invoice and
// payment written before rows named one. It is the precondition for a NOT NULL
// check on customer_link_id.
func customerLinksDataTask() datatask.Task {
	return datatask.Task{
		ID: "core/1794458739_customer_link_backfill", Version: 1,
		Title: systemtext.Literal("datatask.title.core.1794458739_customer_link_backfill"), Requires: []string{"core/1794469011"},
		Steps: []datatask.Step{
			customerLinkStep("invoices", finance.MirrorInvoices),
			customerLinkStep("payments", finance.MirrorPayments),
		},
	}
}

// customerLinkStep walks one mirrored table by id in bounded pages.
func customerLinkStep(name string, table finance.MirrorTable) datatask.Step {
	return datatask.Step{
		Name: name, Limit: 200, MinLimit: 10,
		Audit: datatask.AuditPolicy{PerRecord: true},
		Estimate: func(ctx context.Context, q datatask.Querier, maximum int64) (int64, bool, error) {
			if err := auth.Require(ctx, "data_task", principal.ActionRead); err != nil {
				return 0, false, err
			}
			count, err := finance.CountUnlinkedMirrorRows(ctx, q, table, maximum)
			return min(count, maximum), count > maximum, err
		},
		Batch: func(ctx context.Context, tx pgx.Tx, in datatask.BatchIn) (datatask.BatchOut, error) {
			after, err := datatask.UUIDCursor(in.Cursor)
			if err != nil {
				return datatask.BatchOut{}, err
			}
			inspected, named, err := finance.BackfillCustomerLinksTx(ctx, tx, table, after, in.Limit)
			if err != nil {
				return datatask.BatchOut{}, err
			}
			return datatask.AdvanceUUID(in, inspected, named)
		},
	}
}

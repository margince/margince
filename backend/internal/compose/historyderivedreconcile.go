// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"

	"github.com/margince/margince/backend/internal/modules/search"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func (w *captureBackfillReconcileWorker) reconcileDerivedWork(ctx context.Context) error {
	ctx = principal.SystemActing(ctx, "system:capture")
	more, err := releaseTerminalImportWork(ctx, w.pool)
	if err != nil {
		return err
	}
	graphPending := false
	if err := database.WithWorkspaceTx(ctx, w.pool, func(tx pgx.Tx) error {
		var err error
		graphPending, err = search.HasPendingGraphWorkTx(ctx, tx)
		return err
	}); err != nil {
		return err
	}
	signaturePending := false
	if w.enrichEnabled {
		work, err := newContactsStore(InstallationDB(w.pool)).PendingSignatureWork(ctx, 1)
		if err != nil {
			return err
		}
		signaturePending = len(work) > 0
	}
	if !graphPending && !signaturePending {
		if more {
			return river.JobSnooze(0)
		}
		return nil
	}
	client, err := river.ClientFromContextSafely[pgx.Tx](ctx)
	if err != nil {
		return err
	}
	var graphErr, signatureErr error
	if graphPending {
		_, graphErr = client.Insert(ctx, GraphEdgeReconcileArgs{Pending: true}, pendingWorkOpts(GraphEdgeReconcileArgs{}.Kind()))
	}
	if signaturePending {
		_, signatureErr = client.Insert(ctx, CaptureEnrichArgs{Pending: true}, pendingWorkOpts(CaptureEnrichArgs{}.Kind()))
	}
	if err := errors.Join(graphErr, signatureErr); err != nil {
		return err
	}
	if more {
		return river.JobSnooze(0)
	}
	return nil
}

// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/modules/consent"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/platform/datatask"
)

// Capture treats an unknown provider date as no evidence during partial progress.
func heldMailDataTask(now func() time.Time) datatask.Task {
	return datatask.Task{
		ID: "core/1790871111_mail_already_held_owes_no_notice", Version: 1,
		Title: "Correcting privacy notices for older mail", Requires: []string{"core/1790871110"},
		Steps: []datatask.Step{
			{
				Name: "stamp_provider_received_at", Limit: 200, MinLimit: 10,
				Audit:    datatask.AuditPolicy{Reason: "capture.recordImportTx stamps the same derived provider date without an audit"},
				Estimate: capture.EstimateHeldMail, Batch: capture.StampHeldMailTx,
			},
			{
				Name: "relabel_held_mail_and_close_notices", Limit: 200, MinLimit: 10,
				Audit: datatask.AuditPolicy{PerRecord: true}, Estimate: contacts.EstimateHeldMailAcquisitions,
				Batch: func(ctx context.Context, tx pgx.Tx, in datatask.BatchIn) (datatask.BatchOut, error) {
					acquisitions, out, err := contacts.RelabelHeldMailTx(ctx, tx, in)
					if err != nil {
						return datatask.BatchOut{}, err
					}
					if _, err := consent.SettleHeldMailTx(ctx, tx, acquisitions, now()); err != nil {
						return datatask.BatchOut{}, err
					}
					return out, nil
				},
			},
		},
	}
}

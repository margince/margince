// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/config"
	"github.com/margince/margince/backend/internal/platform/datatask"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// signatureTitleHistoryDataTask reopens the contacts the one-mail signature
// pass left without a title although they wrote more than one readable mail.
// The next pass reads them again, older mail included. It runs after the mail
// body task, so it judges mail by its assessed completeness.
func signatureTitleHistoryDataTask(pool *pgxpool.Pool) datatask.Task {
	return datatask.Task{
		ID: "core/1792799323_signature_title_history", Version: 1,
		Title: "Reading older mail for contacts without a job title", Requires: []string{"core/1792799323"},
		Steps: []datatask.Step{{
			Name: "reopen_untitled_signature_reads", Limit: 200, MinLimit: 10,
			Audit:    datatask.AuditPolicy{Reason: "contacts.MarkSignatureRead writes the same read watermark without an audit"},
			Estimate: estimateOneMailSignatureWatermarks, Batch: reopenSignatureTitleHistoryBatch(pool),
		}},
	}
}

func reopenSignatureTitleHistoryBatch(pool *pgxpool.Pool) func(context.Context, pgx.Tx, datatask.BatchIn) (datatask.BatchOut, error) {
	return func(ctx context.Context, tx pgx.Tx, in datatask.BatchIn) (datatask.BatchOut, error) {
		if err := auth.RequireSystem(ctx); err != nil {
			return datatask.BatchOut{}, err
		}
		after, err := datatask.UUIDCursor(in.Cursor)
		if err != nil {
			return datatask.BatchOut{}, err
		}
		defaultEnrich, err := config.StoredValTx(ctx, tx, StoredConfig(pool), capture.SignatureEnrich)
		if err != nil {
			return datatask.BatchOut{}, err
		}
		page, err := contacts.ReopenSignatureTitleHistoryTx(ctx, tx, after, in.Limit, defaultEnrich)
		if err != nil {
			return datatask.BatchOut{}, err
		}
		return datatask.AdvanceUUID(in, page.Scanned, page.Reopened)
	}
}

func estimateOneMailSignatureWatermarks(ctx context.Context, q datatask.Querier, maximum int64) (int64, bool, error) {
	if err := auth.Require(ctx, "data_task", principal.ActionRead); err != nil {
		return 0, false, err
	}
	count, err := contacts.CountOneMailSignatureWatermarks(ctx, q, maximum)
	return min(count, maximum), count > maximum, err
}

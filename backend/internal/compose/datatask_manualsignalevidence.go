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

// manualSignalNoEvidenceDataTask replaces the sentence the lead form used to
// invent for a signal saved without evidence. The new marker is the one the
// screen shows as "No evidence recorded".
func manualSignalNoEvidenceDataTask() datatask.Task {
	return datatask.Task{
		ID: "core/1792809942_manual_signal_no_evidence", Version: 1,
		Title: "Marking lead signals saved without evidence", Requires: []string{"core/1792799323"},
		Steps: []datatask.Step{{
			Name: "mark_unevidenced_signals", Limit: 200, MinLimit: 10,
			Audit:    datatask.AuditPolicy{PerRecord: true},
			Estimate: estimateUnevidencedManualSignals, Batch: markUnevidencedManualSignalsBatch,
		}},
	}
}

func markUnevidencedManualSignalsBatch(ctx context.Context, tx pgx.Tx, in datatask.BatchIn) (datatask.BatchOut, error) {
	after, err := datatask.UUIDCursor(in.Cursor)
	if err != nil {
		return datatask.BatchOut{}, err
	}
	scanned, changed, err := contacts.MarkUnevidencedManualSignalsTx(ctx, tx, after, in.Limit)
	if err != nil {
		return datatask.BatchOut{}, err
	}
	return datatask.AdvanceUUID(in, scanned, changed)
}

func estimateUnevidencedManualSignals(ctx context.Context, q datatask.Querier, maximum int64) (int64, bool, error) {
	if err := auth.Require(ctx, "data_task", principal.ActionRead); err != nil {
		return 0, false, err
	}
	count, err := contacts.CountUnevidencedManualSignals(ctx, q, maximum)
	return min(count, maximum), count > maximum, err
}

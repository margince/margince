// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/consent"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/modules/privacy"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/datatask"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// requestProofDataTask gives every privacy request closed before proofs were
// written a proof rebuilt from the request row and audit_log. It writes only
// new rows and marks what the records cannot show as unknown, so it runs
// without an operator's review.
func requestProofDataTask() datatask.Task {
	step := datatask.Step{
		Name: "privacy_request_proof", Limit: 200, MinLimit: 1,
		Audit: datatask.AuditPolicy{PerRecord: true},
		Estimate: func(ctx context.Context, q datatask.Querier, maximum int64) (int64, bool, error) {
			if err := auth.Require(ctx, "data_task", principal.ActionRead); err != nil {
				return 0, false, err
			}
			count, err := consent.CountRequestsWithoutProof(ctx, q, maximum)
			return min(count, maximum), count > maximum, err
		},
		Batch: func(ctx context.Context, tx pgx.Tx, in datatask.BatchIn) (datatask.BatchOut, error) {
			if err := auth.RequireSystem(ctx); err != nil {
				return datatask.BatchOut{}, err
			}
			after, err := datatask.UUIDCursor(in.Cursor)
			if err != nil {
				return datatask.BatchOut{}, err
			}
			page, err := consent.BackfillRequestProofsTx(ctx, tx, after, in.Limit, consent.ProofBackfillSeams{
				Zone: identity.ZoneAppliedTx, Erasure: privacy.CertifiedErasureTx,
			})
			if err != nil {
				return datatask.BatchOut{}, err
			}
			return datatask.AdvanceUUID(in, page.Scanned, page.Changed)
		},
	}
	return datatask.Task{
		ID: "core/1792887178_privacy_request_proofs", Version: 1,
		Title:    "Recording the proof of privacy requests answered earlier",
		Requires: []string{"core/1792887178"},
		Steps:    []datatask.Step{step},
	}
}

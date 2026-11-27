// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package consentversions

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/consent"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/datatask"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/systemtext"
)

// DataTask restores historical wording versions from immutable proof.
func DataTask() datatask.Task {
	return datatask.Task{
		ID: "core/1795780000_consent_wording_versions", Version: 1,
		Title:    systemtext.Literal("datatask.title.core.1795780000_consent_wording_versions"),
		Requires: []string{"core/0001"},
		Steps: []datatask.Step{{
			Name: "consent_wording_versions", Limit: 200, MinLimit: 1,
			Audit: datatask.AuditPolicy{Reason: "consent.upsertConsentWithProof derives contact_consent.policy_version from the paired immutable consent_event without a separate version-change audit"},
			Estimate: func(ctx context.Context, q datatask.Querier, maximum int64) (int64, bool, error) {
				if err := auth.Require(ctx, "data_task", principal.ActionRead); err != nil {
					return 0, false, err
				}
				count, err := consent.EstimateConsentVersionBackfill(ctx, q, maximum)
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
				page, err := consent.BackfillConsentVersionsTx(ctx, tx, after, in.Limit)
				if err != nil {
					return datatask.BatchOut{}, err
				}
				return datatask.AdvanceUUID(in, page.Scanned, page.Changed)
			},
		}},
	}
}

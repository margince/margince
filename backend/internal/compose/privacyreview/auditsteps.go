// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package privacyreview

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/modules/notices"
	"github.com/margince/margince/backend/internal/modules/privacy"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/datatask"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// AuditPage corrects one page of audit rows after a cursor and before a run boundary.
type AuditPage func(context.Context, pgx.Tx, ids.UUID, ids.UUID, int, privacy.CountryReader) (privacy.HistoricalAuditPage, error)

// UnlinkedApprovalReviewStep asks the privacy reviewers to check each approval
// history no erasure is proved for. Each audit row that raised a review counts as a change.
func UnlinkedApprovalReviewStep(store *notices.Store) datatask.Step {
	step := AuditSteps(func(ctx context.Context, tx pgx.Tx, after, until ids.UUID, limit int, country privacy.CountryReader) (privacy.HistoricalAuditPage, error) {
		page, err := privacy.UnlinkedApprovalHistoryPageTx(ctx, tx, after, until, limit, country)
		if err != nil || len(page.Review) == 0 {
			return page, err
		}
		flagged, err := FlagTx(ctx, tx, store, UnprovenSubjectScope, page.Review)
		page.Changed += flagged
		return page, err
	})[0]
	step.Name = "unlinked_approval_reviews"
	return step
}

// AuditSteps is the one paged step every audit correction task runs over the
// audit primary key, bounded by the run ID.
func AuditSteps(correct AuditPage) []datatask.Step {
	return []datatask.Step{{
		Name: "audit_payloads", Limit: 200, MinLimit: 1,
		Audit: datatask.AuditPolicy{PerRecord: true},
		Estimate: func(ctx context.Context, q datatask.Querier, maximum int64) (int64, bool, error) {
			if err := auth.Require(ctx, "data_task", principal.ActionRead); err != nil {
				return 0, false, err
			}
			count, err := privacy.CountHistoricalAuditRows(ctx, q, maximum)
			return min(count, maximum), count > maximum, err
		},
		Batch: func(ctx context.Context, tx pgx.Tx, in datatask.BatchIn) (datatask.BatchOut, error) {
			after, err := datatask.UUIDCursor(in.Cursor)
			if err != nil {
				return datatask.BatchOut{}, err
			}
			page, err := correct(ctx, tx, after, in.RunID, in.Limit, identity.CountryForDecision)
			if err != nil {
				return datatask.BatchOut{}, err
			}
			return datatask.AdvanceUUID(in, page.Scanned, page.Changed)
		},
	}}
}

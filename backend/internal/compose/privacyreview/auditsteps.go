// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package privacyreview

import (
	"context"
	"errors"

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

// ReviewerMissing is the action a review step leaves when nobody may review a source.
const ReviewerMissing = "privacy_reviewer_missing"

// UnlinkedApprovalReviewStep asks the privacy reviewers to check each approval
// history no erasure is proved for. Each audit row that raised a review counts as a change.
// With no reviewer the step still finishes and names each history as action needed.
func UnlinkedApprovalReviewStep(store *notices.Store, rbac identity.RbacObjects) datatask.Step {
	step := auditStep(func(ctx context.Context, tx pgx.Tx, after, until ids.UUID, limit int) (privacy.HistoricalAuditPage, *datatask.ActionNeeded, error) {
		page, err := privacy.UnlinkedApprovalHistoryPageTx(ctx, tx, after, until, limit, identity.CountryForDecision)
		if err != nil {
			return page, nil, err
		}
		action := &datatask.ActionNeeded{Reason: ReviewerMissing}
		for _, source := range page.Review {
			flagged, err := FlagTx(ctx, tx, rbac, store, UnprovenSubjectScope, []privacy.ErasureReviewSource{source})
			if _, unavailable := errors.AsType[*UnavailableError](err); unavailable {
				action.Records = append(action.Records, datatask.RecordRef{Type: source.Kind, ID: source.ID})
				continue
			}
			if err != nil {
				return page, nil, err
			}
			page.Changed += flagged
		}
		return page, action, nil
	})
	step.Name = "unlinked_approval_reviews"
	return step
}

// AuditSteps is the one paged step every audit correction task runs over the
// audit primary key, bounded by the run ID.
func AuditSteps(correct AuditPage) []datatask.Step {
	return []datatask.Step{auditStep(func(ctx context.Context, tx pgx.Tx, after, until ids.UUID, limit int) (privacy.HistoricalAuditPage, *datatask.ActionNeeded, error) {
		page, err := correct(ctx, tx, after, until, limit, identity.CountryForDecision)
		return page, nil, err
	})}
}

type auditBatch func(context.Context, pgx.Tx, ids.UUID, ids.UUID, int) (privacy.HistoricalAuditPage, *datatask.ActionNeeded, error)

func auditStep(correct auditBatch) datatask.Step {
	return datatask.Step{
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
			page, action, err := correct(ctx, tx, after, in.RunID, in.Limit)
			if err != nil {
				return datatask.BatchOut{}, err
			}
			out, err := datatask.AdvanceUUID(in, page.Scanned, page.Changed)
			out.Action = action
			return out, err
		},
	}
}

// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"context"
	"strings"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// ListActivitiesTx keeps a composite read inside its existing transaction.
func ListActivitiesTx(ctx context.Context, tx pgx.Tx, in ListActivitiesInput) ([]crmcontracts.Activity, storekit.Page, error) {
	if err := requireActivityListScope(ctx, tx, in); err != nil {
		return nil, storekit.Page{}, err
	}
	var err error
	if in.mailIdentities, err = currentTaskMailIdentities(ctx, tx, in); err != nil {
		return nil, storekit.Page{}, err
	}
	limit := storekit.ClampLimit(in.Limit)
	where, content, sorted, args, err := listActivitiesFilter(ctx, in)
	if err != nil {
		return nil, storekit.Page{}, err
	}
	order := sorted.OrderBy()
	if in.RequestReviewAsOf != nil || in.RequestCurrentAsOf != nil {
		order = requestPriorityOrder
	}

	rows, err := tx.Query(ctx,
		`SELECT `+activityColumns(content)+sorted.CursorKeySuffix()+
			` FROM activity a WHERE `+strings.Join(where, " AND ")+
			order+sprintf(" LIMIT %d", limit+1),
		waitingListArgs(in, args)...)
	if err != nil {
		return nil, storekit.Page{}, err
	}
	// Collected rather than streamed: AttachLinks runs a second query on
	// this same transaction, which needs the cursor already closed.
	type pagedRow struct {
		activity crmcontracts.Activity
		key      *string
	}
	paged, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (pagedRow, error) {
		activity, key, err := scanActivityPage(row, sorted)
		return pagedRow{activity, key}, err
	})
	if err != nil {
		return nil, storekit.Page{}, err
	}
	activities := make([]crmcontracts.Activity, 0, len(paged))
	for _, row := range paged {
		activities = append(activities, row.activity)
	}
	var page storekit.Page
	if len(activities) > limit {
		activities = activities[:limit]
		if in.RequestReviewAsOf == nil && in.RequestCurrentAsOf == nil {
			last := activities[limit-1]
			next, err := sorted.EncodePageCursor(paged[limit-1].key, last.CreatedAt, ids.UUID(last.Id))
			if err != nil {
				return nil, storekit.Page{}, err
			}
			page = storekit.Page{HasMore: true, NextCursor: next}
		}
		// Request priority has no resumable cursor; its count still includes later rows.
	}
	if err := WithEmailRowFacts(ctx, tx, activities); err != nil {
		return nil, storekit.Page{}, err
	}
	if err := AttachLinks(ctx, tx, activities); err != nil {
		return nil, storekit.Page{}, err
	}
	if err := WithOffersEmail(ctx, tx, activities); err != nil {
		return nil, storekit.Page{}, err
	}
	if err := WithCommitmentAdmissions(ctx, tx, activities); err != nil {
		return nil, storekit.Page{}, err
	}
	if err := StampActivitiesWritable(ctx, tx, activities); err != nil {
		return nil, storekit.Page{}, err
	}
	if err := withCustomValues(ctx, tx, in.Columns.cols, pageOf(activities)); err != nil {
		return nil, storekit.Page{}, err
	}
	return activities, page, nil
}

func requireActivityListScope(ctx context.Context, tx pgx.Tx, in ListActivitiesInput) error {
	if err := auth.Require(ctx, "activity", principal.ActionRead); err != nil {
		return err
	}
	if in.AssigneeID != nil && (in.TaskAsOf == nil || in.TaskAsOf.IsZero()) {
		return apperrors.ErrInvalidArgument
	}

	// A visible sibling link cannot grant access to the requested target.
	if err := ensureNarrowingTargetVisible(ctx, tx, in.EntityType, in.EntityID); err != nil {
		return err
	}
	if in.WithinProjectID != nil {
		if err := RequireProjectScope(ctx, tx, *in.WithinProjectID); err != nil {
			return err
		}
	}
	if in.SourceActivityID != nil {
		if _, err := readActivityContent(ctx, tx, *in.SourceActivityID, storekit.IncludeArchived); err != nil {
			return err
		}
	}
	return nil
}

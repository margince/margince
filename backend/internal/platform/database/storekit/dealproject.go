// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package storekit

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/relstrength"
)

// DealsProject is the one live project the live deals roll up to, among the
// projects the caller may read. It answers zero when no deal has one. It also
// answers zero for deals of two projects, or a caller with no project read.
// Every writer that files an activity on a deal asks it. A note, a send and a
// captured mail on one deal so land on the same project.
func DealsProject(ctx context.Context, tx pgx.Tx, deals []ids.UUID) (ids.UUID, error) {
	if len(deals) == 0 {
		return ids.Nil, nil
	}
	return projectOfDeals(ctx, tx, "d.id = ANY($1)", deals)
}

// ActivityDealsProject is DealsProject over the deals the activity is filed on
// and not marked unrelated to.
func ActivityDealsProject(ctx context.Context, tx pgx.Tx, activity ids.ActivityID) (ids.UUID, error) {
	return projectOfDeals(ctx, tx, `d.id IN (SELECT l.deal_id FROM activity_link l
		 WHERE l.activity_id = $1 AND l.entity_type = 'deal' AND `+relstrength.DealLinkStandsSQL("l")+`)`, activity)
}

// projectOfDeals reads the one project of the deals dealsWhere selects; $1 is
// bound to subject.
func projectOfDeals[S []ids.UUID | ids.ActivityID](ctx context.Context, tx pgx.Tx, dealsWhere string, subject S) (ids.UUID, error) {
	if err := auth.Require(ctx, "project", principal.ActionRead); err != nil {
		if errors.Is(err, apperrors.ErrPermissionDenied) {
			return ids.Nil, nil
		}
		return ids.Nil, err
	}
	args := []any{subject}
	arg := func(v any) int { args = append(args, v); return len(args) }
	scope, err := auth.ScopeClauseFor(ctx, "project", "p", arg)
	if err != nil {
		return ids.Nil, err
	}
	if scope != "" {
		scope = " AND " + scope
	}
	rows, err := tx.Query(ctx, `
		SELECT DISTINCT p.id
		  FROM deal d
		  JOIN project p ON p.id = d.project_id
		 WHERE `+dealsWhere+` AND d.archived_at IS NULL
		   AND p.archived_at IS NULL`+scope+`
		 ORDER BY p.id
		 LIMIT 2`, args...)
	if err != nil {
		return ids.Nil, fmt.Errorf("reading the deals' project: %w", err)
	}
	matched, err := pgx.CollectRows(rows, pgx.RowTo[ids.UUID])
	if err != nil {
		return ids.Nil, fmt.Errorf("reading the deals' project: %w", err)
	}
	if len(matched) != 1 {
		return ids.Nil, nil
	}
	return matched[0], nil
}

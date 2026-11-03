// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// DealActivityList retains the limit for each deal, rather than cutting a shared page.
type DealActivityList struct {
	Activities []crmcontracts.Activity
	Truncated  bool
}

// ListActivitiesByDealTx uses the timeline's admission and enrichment for a batch.
func ListActivitiesByDealTx(ctx context.Context, tx pgx.Tx, dealIDs []ids.UUID, in ListActivitiesInput) (map[ids.UUID]DealActivityList, error) {
	if err := requireActivityListScope(ctx, tx, in); err != nil {
		return nil, err
	}
	if err := auth.Require(ctx, "deal", principal.ActionRead); err != nil {
		return nil, err
	}
	var err error
	if in.mailIdentities, err = currentTaskMailIdentities(ctx, tx, in); err != nil {
		return nil, err
	}
	where, content, sorted, args, err := listActivitiesFilter(ctx, in)
	if err != nil {
		return nil, err
	}
	arg := func(v any) int { args = append(args, v); return len(args) }
	deals, err := readableDealBatchSQL(ctx, dealIDs, arg)
	if err != nil {
		return nil, err
	}
	where = append(where, `EXISTS (SELECT 1 FROM activity_link on_deal
		WHERE on_deal.activity_id = a.id AND on_deal.deal_id = d.id)`)
	order := sorted.OrderBy()
	if in.RequestReviewAsOf != nil || in.RequestCurrentAsOf != nil {
		order = requestPriorityOrder
	}
	limit := storekit.ClampLimit(in.Limit)
	var lists map[ids.UUID]DealActivityList
	err = perBind(ctx, in.WaitingReplyAsOf != nil, func(ctx context.Context) error {
		rows, err := tx.Query(ctx, `SELECT `+activityColumns(content)+`, d.id AS linked_deal_id
		FROM deal d CROSS JOIN LATERAL (SELECT a.id FROM activity a
		WHERE `+strings.Join(where, " AND ")+order+fmt.Sprintf(" LIMIT %d", limit+1)+`) selected JOIN activity a ON a.id = selected.id
		WHERE `+deals+` ORDER BY d.id, `+strings.TrimPrefix(order, " ORDER BY "), waitingListArgs(in, args)...)
		if err != nil {
			return err
		}
		lists, err = collectDealActivityLists(ctx, tx, rows, in.Columns, listClock(in), limit)
		return err
	})
	return lists, err
}

func readableDealBatchSQL(ctx context.Context, dealIDs []ids.UUID, arg func(any) int) (string, error) {
	where := fmt.Sprintf("d.id = ANY($%d) AND d.archived_at IS NULL", arg(dealIDs))
	scope, err := auth.ScopeClauseFor(ctx, "deal", "d", arg)
	if scope != "" {
		where += " AND " + scope
	}
	return where, err
}

func collectDealActivityLists(ctx context.Context, tx pgx.Tx, rows pgx.Rows, columns CustomColumns, asOf time.Time, limit int) (map[ids.UUID]DealActivityList, error) {
	defer rows.Close()
	byDeal := map[ids.UUID][]ids.UUID{}
	positions := map[ids.UUID]int{}
	var unique []crmcontracts.Activity
	for rows.Next() {
		var dealID ids.UUID
		activity, err := scanActivity(storekit.TrailingColumns{Row: rows, Dest: []any{&dealID}})
		if err != nil {
			return nil, err
		}
		id := ids.UUID(activity.Id)
		byDeal[dealID] = append(byDeal[dealID], id)
		if _, seen := positions[id]; !seen {
			positions[id] = len(unique)
			unique = append(unique, activity)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	rows.Close()
	if err := finishActivityList(ctx, tx, columns, unique, asOf); err != nil {
		return nil, err
	}
	out := make(map[ids.UUID]DealActivityList, len(byDeal))
	for dealID, wanted := range byDeal {
		page := DealActivityList{Truncated: len(wanted) > limit}
		for _, id := range wanted[:min(len(wanted), limit)] {
			page.Activities = append(page.Activities, unique[positions[id]])
		}
		out[dealID] = page
	}
	return out, nil
}

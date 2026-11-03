// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/occurredday"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// DealOpenTasks keeps each deal's own truncation state.
type DealOpenTasks struct {
	Tasks     []OpenTask
	Truncated bool
}

// OpenTasksByDealTx reads the same current promises as ListOpenTasksTx, per deal.
func OpenTasksByDealTx(ctx context.Context, tx pgx.Tx, dealIDs []ids.UUID, in ListOpenTasksInput) (map[ids.UUID]DealOpenTasks, error) {
	if err := auth.Require(ctx, "activity", principal.ActionRead); err != nil {
		return nil, err
	}
	if err := auth.Require(ctx, "deal", principal.ActionRead); err != nil {
		return nil, err
	}
	var args []any
	arg := func(v any) int { args = append(args, v); return len(args) }
	identities, err := storekit.MailSeatIdentitiesSQL(ctx, tx, arg)
	if err != nil {
		return nil, err
	}
	where, err := openTasksFilter(ctx, in, identities, arg)
	if err != nil {
		return nil, err
	}
	deals, err := readableDealBatchSQL(ctx, dealIDs, arg)
	if err != nil {
		return nil, err
	}
	where = append(where, `EXISTS (SELECT 1 FROM activity_link on_deal
		WHERE on_deal.activity_id = a.id AND on_deal.deal_id = d.id)`)
	limit := openTasksLimit(in.Limit)
	rows, err := tx.Query(ctx, `SELECT d.id, `+openTaskColumns()+`
		FROM deal d CROSS JOIN LATERAL (SELECT a.id FROM activity a
		WHERE `+joinAnd(where)+` ORDER BY `+openTasksOrder(in.MostRecentlySlippedFirst)+
		fmt.Sprintf(" LIMIT %d", limit+1)+`) selected JOIN activity a ON a.id = selected.id LEFT JOIN app_user u ON u.id = a.assignee_id
		WHERE `+deals+` ORDER BY d.id, `+openTasksOrder(in.MostRecentlySlippedFirst), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[ids.UUID]DealOpenTasks{}
	for rows.Next() {
		var dealID ids.UUID
		var task OpenTask
		if err := rows.Scan(&dealID, &task.ID, &task.Subject, &task.DueAt, &task.AssigneeID, &task.AssigneeName, &task.CreatedAt); err != nil {
			return nil, err
		}
		page := out[dealID]
		if len(page.Tasks) < limit {
			page.Tasks = append(page.Tasks, task)
		} else {
			page.Truncated = true
		}
		out[dealID] = page
	}
	return out, rows.Err()
}

// NextBookedMeetingsByDealTx checks each deal through the shared booked predicate.
func NextBookedMeetingsByDealTx(ctx context.Context, tx pgx.Tx, dealIDs []ids.UUID, after time.Time) (map[ids.UUID]BookedMeeting, error) {
	if err := auth.Require(ctx, "activity", principal.ActionRead); err != nil {
		return nil, err
	}
	if err := auth.Require(ctx, "deal", principal.ActionRead); err != nil {
		return nil, err
	}
	var args []any
	arg := func(v any) int { args = append(args, v); return len(args) }
	where, err := bookedMeetingWhere(ctx, after, arg)
	if err != nil {
		return nil, err
	}
	deals, err := readableDealBatchSQL(ctx, dealIDs, arg)
	if err != nil {
		return nil, err
	}
	where = append(where, `EXISTS (SELECT 1 FROM activity_link on_deal
		WHERE on_deal.activity_id = a.id AND on_deal.deal_id = d.id)`)
	rows, err := tx.Query(ctx, `SELECT d.id, a.id, coalesce(a.subject, ''), a.occurred_at, a.occurred_precision
		FROM deal d CROSS JOIN LATERAL (SELECT a.id FROM activity a
		WHERE `+joinAnd(where)+` ORDER BY `+occurredday.StartSQL("a")+`, a.id LIMIT 1) selected JOIN activity a ON a.id = selected.id
		WHERE `+deals, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[ids.UUID]BookedMeeting{}
	for rows.Next() {
		var dealID ids.UUID
		var meeting BookedMeeting
		if err := rows.Scan(&dealID, &meeting.ID, &meeting.Subject, &meeting.StartsAt, &meeting.Precision); err != nil {
			return nil, err
		}
		out[dealID] = meeting
	}
	return out, rows.Err()
}

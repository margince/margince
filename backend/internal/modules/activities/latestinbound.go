// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/occurredday"
	"github.com/margince/margince/backend/internal/shared/kernel/provenance"
)

// JustInDays is how far back a record page leads with what arrived. Older
// unanswered mail belongs to the Worklist's waiting lane, which ranks it.
const JustInDays = 7

const latestInboundLimit = 10

// LatestInbound reads what they sent on one record in the last JustInDays that
// nobody has answered, newest first, and the record's last exchange. Mail and
// messages come from the waiting_reply read, so this never judges a thread on its own.
func (s *Store) LatestInbound(ctx context.Context, entityType string, entityID ids.UUID) (crmcontracts.LatestInboundList, error) {
	columns, err := s.ActiveActivityColumns(ctx)
	if err != nil {
		return crmcontracts.LatestInboundList{}, err
	}
	now := s.now()
	since := now.Add(-JustInDays * 24 * time.Hour)
	var out crmcontracts.LatestInboundList
	err = s.tx(ctx, func(tx pgx.Tx) error {
		// An unknown record is a 404, which the timeline's own gate serves as an empty list.
		if err := auth.EnsureVisibleExisting(ctx, tx, entityType, entityID); err != nil {
			return err
		}
		read := func(in ListActivitiesInput) ([]crmcontracts.Activity, error) {
			in.EntityType, in.EntityID, in.AsOf, in.Columns = &entityType, &entityID, &now, columns
			rows, _, err := s.ListActivitiesIn(ctx, tx, in)
			return rows, err
		}
		limit, one, note, form := latestInboundLimit, 1, string(crmcontracts.ActivityKindNote), provenance.WebFormSource
		waiting, err := read(ListActivitiesInput{OccurredAfter: &since, WaitingReply: true, Limit: &limit})
		if err != nil {
			return err
		}
		forms, err := read(ListActivitiesInput{OccurredAfter: &since, Kind: &note, SourceSystem: &form, Limit: &limit})
		if err != nil {
			return err
		}
		answer, err := read(ListActivitiesInput{AnswersThem: true, Limit: &one})
		if err != nil {
			return err
		}
		if out.Data, err = markLatestInboundTasks(ctx, tx, unansweredInbound(waiting, forms, answer), now); err != nil {
			return err
		}
		out.LastExchange, err = s.lastExchange(ctx, tx, entityType, entityID, now, columns)
		return err
	})
	return out, err
}

// unansweredInbound merges the waiting mail and messages with the website
// forms no later answer follows, newest first and at most latestInboundLimit.
// newestAnswer is the record's newest row that answers them, if any.
func unansweredInbound(waiting, forms, newestAnswer []crmcontracts.Activity) []crmcontracts.LatestInbound {
	items := []crmcontracts.LatestInbound{}
	for _, row := range waiting {
		switch row.Kind {
		case crmcontracts.ActivityKindEmail:
			items = append(items, crmcontracts.LatestInbound{Activity: row, Kind: crmcontracts.LatestInboundKindEmail})
		case crmcontracts.ActivityKindMessage:
			items = append(items, crmcontracts.LatestInbound{Activity: row, Kind: crmcontracts.LatestInboundKindMessage})
		}
	}
	for _, row := range forms {
		if len(newestAnswer) == 0 || !newestAnswer[0].OccurredAt.After(row.OccurredAt) {
			items = append(items, crmcontracts.LatestInbound{Activity: row, Kind: crmcontracts.LatestInboundKindWebForm})
		}
	}
	sort.SliceStable(items, func(i, j int) bool {
		return items[i].Activity.OccurredAt.After(items[j].Activity.OccurredAt)
	})
	if len(items) > latestInboundLimit {
		items = items[:latestInboundLimit]
	}
	return items
}

// answersThemSQL keeps a row under alias a that answers them. That is anything
// we sent, a call, or a meeting that was not canceled or a no-show. A note answers nobody.
const answersThemSQL = `((a.kind IN ('email', 'message') AND a.direction = 'outbound') OR a.kind = 'call'
 OR (a.kind = 'meeting' AND a.meeting_status IS DISTINCT FROM 'canceled' AND a.meeting_status IS DISTINCT FROM 'no_show'))`

// markLatestInboundTasks drops an item a task already stands for, which
// waiting_reply does for mail and a form needs here, and sets accepts_request.
// That test is the one takeEmailRequest makes, so an item offered as accepting
// its request is one the server accepts.
func markLatestInboundTasks(ctx context.Context, tx pgx.Tx, items []crmcontracts.LatestInbound, now time.Time) ([]crmcontracts.LatestInbound, error) {
	if len(items) == 0 {
		return items, nil
	}
	wanted := make([]ids.UUID, len(items))
	for i, item := range items {
		wanted[i] = ids.UUID(item.Activity.Id)
	}
	args := []any{}
	arg := func(value any) int { args = append(args, value); return len(args) }
	in, instant := arg(wanted), fmt.Sprintf("$%d", arg(now))
	// A task that is no longer current work stands for nothing.
	current := storekit.CurrentTaskClause("task", now, "'[]'::jsonb", arg)
	rows, err := tx.Query(ctx, storekit.SQLf(`SELECT a.id,
 %s OR EXISTS (SELECT 1 FROM activity task WHERE task.kind = 'task'
   AND task.source_activity_id = a.id AND NOT task.is_done AND task.archived_at IS NULL AND %s),
 coalesce((%s) AND %s, false)
 FROM activity a WHERE a.id = ANY($%d)`, openRequestReminderSQL, current, reviewableRequestSQL(instant),
		occurredday.BegunSQL("a", instant), in), args...)
	if err != nil {
		return nil, fmt.Errorf("activities: reading what stands for the latest inbound: %w", err)
	}
	defer rows.Close()
	type mark struct{ hasTask, acceptsRequest bool }
	marks := map[ids.UUID]mark{}
	for rows.Next() {
		var id ids.UUID
		var m mark
		if err := rows.Scan(&id, &m.hasTask, &m.acceptsRequest); err != nil {
			return nil, err
		}
		marks[id] = m
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	open := items[:0]
	for _, item := range items {
		m := marks[ids.UUID(item.Activity.Id)]
		if m.hasTask {
			continue
		}
		// Accepting reads the request's content, which a withheld row refuses.
		withheld := item.Activity.ContentState != nil &&
			*item.Activity.ContentState == crmcontracts.ActivityContentStateWithheld
		item.AcceptsRequest = m.acceptsRequest && !withheld
		open = append(open, item)
	}
	return open, nil
}

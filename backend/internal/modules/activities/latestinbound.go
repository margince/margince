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
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/occurredday"
	"github.com/margince/margince/backend/internal/shared/kernel/provenance"
)

// JustInDays is how far back a record page leads with what arrived. Older
// unanswered mail belongs to the Worklist's waiting lane, which ranks it.
const JustInDays = 7

const (
	latestInboundLimit = 10
	// A week of one record's rows: the forms and what answered them.
	latestInboundRecentLimit = 50
)

// LatestInbound reads what they sent on one record in the last JustInDays that
// nobody has answered, newest first. Mail and messages come from the
// waiting_reply read, so this never judges a thread on its own.
func (s *Store) LatestInbound(ctx context.Context, entityType string, entityID ids.UUID) ([]crmcontracts.LatestInbound, error) {
	columns, err := s.ActiveActivityColumns(ctx)
	if err != nil {
		return nil, err
	}
	now := s.now()
	since := now.Add(-JustInDays * 24 * time.Hour)
	var out []crmcontracts.LatestInbound
	err = s.tx(ctx, func(tx pgx.Tx) error {
		read := func(waiting bool, limit int) ([]crmcontracts.Activity, error) {
			rows, _, err := s.ListActivitiesIn(ctx, tx, ListActivitiesInput{
				EntityType: &entityType, EntityID: &entityID, OccurredAfter: &since,
				WaitingReply: waiting, Limit: &limit, AsOf: &now, Columns: columns,
			})
			return rows, err
		}
		waiting, err := read(true, latestInboundLimit)
		if err != nil {
			return err
		}
		recent, err := read(false, latestInboundRecentLimit)
		if err != nil {
			return err
		}
		out, err = markLatestInboundTasks(ctx, tx, unansweredInbound(waiting, recent), now)
		return err
	})
	return out, err
}

// unansweredInbound merges the waiting mail and messages with the website
// forms nothing answered yet, newest first and at most latestInboundLimit.
func unansweredInbound(waiting, recent []crmcontracts.Activity) []crmcontracts.LatestInbound {
	items := []crmcontracts.LatestInbound{}
	for _, row := range waiting {
		switch row.Kind {
		case crmcontracts.ActivityKindEmail:
			items = append(items, crmcontracts.LatestInbound{Activity: row, Kind: crmcontracts.LatestInboundKindEmail})
		case crmcontracts.ActivityKindMessage:
			items = append(items, crmcontracts.LatestInbound{Activity: row, Kind: crmcontracts.LatestInboundKindMessage})
		}
	}
	for _, row := range recent {
		if isWebForm(row) && !answeredAfter(recent, row.OccurredAt) {
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

func isWebForm(row crmcontracts.Activity) bool {
	return row.Kind == crmcontracts.ActivityKindNote && row.SourceSystem != nil &&
		*row.SourceSystem == provenance.WebFormSource
}

// answeredAfter reports a row that answers them later than at: anything we
// sent, or a call or meeting held with them. A note answers nobody.
func answeredAfter(rows []crmcontracts.Activity, at time.Time) bool {
	for _, row := range rows {
		if row.OccurredAt.After(at) && answersThem(row) {
			return true
		}
	}
	return false
}

func answersThem(row crmcontracts.Activity) bool {
	switch row.Kind {
	case crmcontracts.ActivityKindEmail, crmcontracts.ActivityKindMessage:
		return row.Direction != nil && *row.Direction == crmcontracts.ActivityDirectionOutbound
	case crmcontracts.ActivityKindCall:
		return true
	case crmcontracts.ActivityKindMeeting:
		status := row.MeetingStatus
		return status == nil || (*status != crmcontracts.ActivityMeetingStatusCanceled &&
			*status != crmcontracts.ActivityMeetingStatusNoShow)
	default:
		return false
	}
}

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

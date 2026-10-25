// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package activities

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// writeReminderCase writes one case's task through the store's own writers.
func writeReminderCase(ctx context.Context, t *testing.T, store *Store, e *sendEnv, c reminderCase) ids.ActivityID {
	t.Helper()
	asOf := c.readInstant(t)
	row := c.row(e.rep, e.other, asOf)
	holder := &ids.UserID{}
	switch c.holder {
	case heldByReader:
		*holder = ids.From[ids.UserKind](e.rep)
	case heldByColleague:
		*holder = ids.From[ids.UserKind](e.other)
	case heldByNobody:
		holder = nil
	}
	created, _, err := store.LogActivity(ctx, LogActivityInput{
		Kind: "task", Subject: new(c.name), Source: "manual", RemindAt: row.RemindAt, AssigneeID: holder,
	})
	if err != nil {
		t.Fatalf("writing the task: %v", err)
	}
	id := ids.From[ids.ActivityKind](ids.UUID(created.Id))
	patch := UpdateActivityInput{}
	if c.done {
		patch.IsDone = new(true)
	}
	if holder == nil {
		patch.Clear = []string{"assignee_id"}
	}
	if c.done || holder == nil {
		if _, err := store.UpdateActivity(ctx, id, patch); err != nil {
			t.Fatalf("updating the task: %v", err)
		}
	}
	if c.archived {
		if _, err := store.ArchiveActivity(asArchiver(e), id, nil); err != nil {
			t.Fatalf("archiving the task: %v", err)
		}
	}
	return id
}

// asArchiver is the rep holding the delete grant archiving asks for.
func asArchiver(e *sendEnv) context.Context {
	ctx := principal.WithWorkspaceID(context.Background(), e.ws)
	ctx = principal.WithCorrelationID(ctx, ids.NewV7())
	return principal.WithActor(ctx, principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:" + e.rep.String(), UserID: e.rep,
		Permissions: principal.Permissions{
			RoleKeys: []string{"rep"},
			Objects:  map[string]principal.ObjectGrant{"activity": {Read: true, Update: true, Delete: true}},
			RowScope: principal.RowScopeAll,
		},
	})
}

// The SQL writer of the rule, held to the same table the Go writer is.
func TestTheReminderClauseAnswersTheCaseTable(t *testing.T) {
	e := setupSend(t)
	ctx := e.as(principal.RowScopeAll)
	store := pinStore(e)
	for _, c := range reminderCases {
		t.Run(c.name, func(t *testing.T) {
			asOf := c.readInstant(t)
			id := writeReminderCase(ctx, t, store, e, c)
			var matched bool
			if err := database.WithWorkspaceTx(ctx, e.pool, func(tx pgx.Tx) error {
				args := []any{}
				arg := func(v any) int { args = append(args, v); return len(args) }
				row := arg(id)
				clause := dueReminderClause("a", sprintf("$%d", arg(e.rep)), asOf, arg)
				return tx.QueryRow(ctx, sprintf(`SELECT EXISTS (SELECT 1 FROM activity a WHERE a.id = $%d AND %s)`, row, clause),
					args...).Scan(&matched)
			}); err != nil {
				t.Fatalf("asking the clause: %v", err)
			}
			if matched != c.want {
				t.Errorf("dueReminderClause = %v, want %v", matched, c.want)
			}
		})
	}
}

// The list filter is the reader's own due reminders: the same set, judged by
// the store's own clock, and none of a colleague's.
func TestTheReminderFilterListsTheReadersOwnDueReminders(t *testing.T) {
	e := setupSend(t)
	ctx := e.as(principal.RowScopeAll)
	asOf := reminderInstant(t)
	store := pinStore(e).WithClock(func() time.Time { return asOf })
	want := map[ids.UUID]bool{}
	for _, c := range reminderCases {
		if c.readAt != nil {
			continue
		}
		id := writeReminderCase(ctx, t, store, e, c)
		if c.want {
			want[id.UUID] = true
		}
	}

	rows, _, err := store.ListActivities(ctx, ListActivitiesInput{RemindersDue: true})
	if err != nil {
		t.Fatalf("listing reminders: %v", err)
	}
	got := map[ids.UUID]bool{}
	for _, row := range rows {
		got[ids.UUID(row.Id)] = true
	}
	if len(got) != len(want) {
		t.Fatalf("the filter listed %d reminders, want %d", len(got), len(want))
	}
	for id := range want {
		if !got[id] {
			t.Errorf("the filter left out the due reminder %s", id)
		}
	}
	for i := 1; i < len(rows); i++ {
		if rows[i].RemindAt.After(*rows[i-1].RemindAt) {
			t.Errorf("row %d came due after the row above it; the latest reminder leads", i)
		}
	}

	colleagueRows, _, err := store.ListActivities(asColleague(e), ListActivitiesInput{RemindersDue: true})
	if err != nil {
		t.Fatalf("listing the colleague's reminders: %v", err)
	}
	if len(colleagueRows) != 1 {
		t.Errorf("the colleague is reminded of %d tasks, want only the one they hold", len(colleagueRows))
	}
}

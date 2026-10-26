// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package activities

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// A handover is an update, so a seat that may only read moves nothing and is
// told so. A dead connection fails the move and the count.
func TestHandingOverTasksNeedsTheUpdateGrantAndASoundConnection(t *testing.T) {
	e := setupPromises(t)
	store := NewStore(database.BindTo(e.pool, ids.From[ids.WorkspaceKind](e.ws)))
	from, to := ids.From[ids.UserKind](e.rep), ids.From[ids.UserKind](e.other)

	err := database.BindTo(e.pool, ids.From[ids.WorkspaceKind](e.ws)).Tx(e.asReadSeat(), func(tx pgx.Tx) error {
		_, err := store.HandOverOpenTasksTx(noUpdateGrant(e), tx, from, to)
		return err
	})
	if err == nil || !isDenied(err) {
		t.Fatalf("a caller without the update grant gave %v, want a permission refusal", err)
	}

	cancelled, cancel := context.WithCancel(e.asEditor())
	err = database.BindTo(e.pool, ids.From[ids.WorkspaceKind](e.ws)).Tx(e.asEditor(), func(tx pgx.Tx) error {
		cancel()
		if _, err := store.HandOverOpenTasksTx(cancelled, tx, from, to); err == nil {
			t.Error("a handover on a cancelled context reported success")
		}
		if _, err := store.OpenTasksAssignedTo(cancelled, tx, from); err == nil {
			t.Error("a count on a cancelled context reported success")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func noUpdateGrant(e *promiseEnv) context.Context {
	ctx := e.asEditor()
	actor, _ := principal.Actor(ctx)
	actor.Permissions.Objects["activity"] = principal.ObjectGrant{Read: true}
	return principal.WithActor(ctx, actor)
}

func isDenied(err error) bool { return errors.Is(err, apperrors.ErrPermissionDenied) }

// A caller with workspace-wide read counts every open task the member holds.
func TestAWorkspaceWideReaderCountsAMembersOpenTasks(t *testing.T) {
	e := setupPromises(t)
	task := ids.NewV7()
	e.exec(t, `INSERT INTO activity (id, kind, subject, occurred_at, source, captured_by, assignee_id, is_done)
		VALUES ($1, 'task', 'Call back', now(), 'seed', 'system', $2, false)`, task, e.other)
	ctx := e.asEditor()
	actor, _ := principal.Actor(ctx)
	actor.Permissions.RowScope = principal.RowScopeAll
	ctx = principal.WithActor(ctx, actor)

	store := NewStore(database.BindTo(e.pool, ids.From[ids.WorkspaceKind](e.ws)))
	var counted int
	err := database.BindTo(e.pool, ids.From[ids.WorkspaceKind](e.ws)).Tx(ctx, func(tx pgx.Tx) error {
		var err error
		counted, err = store.OpenTasksAssignedTo(ctx, tx, ids.From[ids.UserKind](e.other))
		return err
	})
	if err != nil || counted != 1 {
		t.Fatalf("open tasks counted = %d, err %v; want 1", counted, err)
	}
}

// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// Setup statements the harness runs DIRECTLY, outside the product's own doors.
//
// Split from harness.go because they are the one part of it that is not a
// fixture: everything there builds a world through the app, while these three
// reach past it to the estate — to seed a shape no endpoint produces, to count
// rows a reader would not be shown, and to assert that a statement the schema
// must refuse is refused.
//
// Every one of them binds the workspace explicitly, through
// database.WithWorkspaceTx. What scopes these statements is that binding — the
// GUC the transaction sets — and the test pool has to set it like any other
// caller, owner-less or not.

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// WsExec runs one setup statement in a workspace-bound transaction (RLS is
// FORCED, so the GUC must be set even for the owner-less test pool).
func (e *Env) WsExec(t *testing.T, sql string, args ...any) {
	t.Helper()
	ctx := principal.WithWorkspaceID(context.Background(), e.WS)
	if err := database.WithWorkspaceTx(ctx, e.Pool, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, sql, args...)
		return err
	}); err != nil {
		t.Fatalf("setup exec: %v", err)
	}
}

// WsExecErr is WsExec for a statement the estate is EXPECTED to refuse: it
// hands the error back rather than failing the test on it, so a suite can
// assert which rule refused and not merely that something did.
func (e *Env) WsExecErr(t *testing.T, sql string, args ...any) error {
	t.Helper()
	ctx := principal.WithWorkspaceID(context.Background(), e.WS)
	return database.WithWorkspaceTx(ctx, e.Pool, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, sql, args...)
		return err
	})
}

// WsCount returns a scalar count in a workspace-bound transaction.
func (e *Env) WsCount(t *testing.T, sql string, args ...any) int {
	t.Helper()
	ctx := principal.WithWorkspaceID(context.Background(), e.WS)
	var n int
	if err := database.WithWorkspaceTx(ctx, e.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, sql, args...).Scan(&n)
	}); err != nil {
		t.Fatalf("count query: %v", err)
	}
	return n
}

// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package apptest

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"
)

// InWorkspace runs fn on the owner connection under the bootstrapped
// installation's workspace GUC. Core carries no row-level security since 0217,
// so the binding scopes nothing there; it is still set because the extension
// tables' FORCE RLS policies read it, and a unit's table is unreachable
// without it.
//
// It takes no slug. One installation serves one organization (ADR-0061), so
// there is one row to find, and ADR-0091 retired the column a caller used to
// name it by. Callers passed their env's slug for years believing it selected
// something.
//
// It lives in apptest rather than beside the suites that call it because it takes
// an AppEnv: the parent integration package's ordinary files cannot import
// apptest without closing an import cycle through compose, so a fixture keyed on
// this type has exactly one home that every suite package can reach.
func InWorkspace(e *AppEnv, t *testing.T, fn func(pgx.Tx) error) error {
	t.Helper()
	ctx := context.Background()
	tx, err := e.Owner.Begin(ctx)
	if err != nil {
		return err
	}
	//craft:ignore swallowed-errors error-path safety net only — the Commit below is asserted, after which this rollback is a designed no-op
	defer func() { _ = tx.Rollback(ctx) }()
	wsID := InstallationWorkspaceID(ctx, t, tx)
	if _, err := tx.Exec(ctx, `SELECT set_config('app.workspace_id', $1, true)`, wsID); err != nil {
		return err
	}
	if err := fn(tx); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

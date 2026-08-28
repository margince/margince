// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package apptest

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"
)

// InWorkspace runs fn on the owner connection inside ONE transaction, so a
// fixture's writes land or roll back together the way a production write does.
//
// It takes no slug: one installation serves one organization (ADR-0061), so
// there is one row to find and nothing for a caller to select between.
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
	if err := fn(tx); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

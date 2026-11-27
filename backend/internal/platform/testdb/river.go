// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package testdb

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

// EnsureRiverSchema layers River's own schema onto the migrated test database,
// at most once per database.
//
// Call it after EnsureSchema: a rebuild opens with DROP SCHEMA public CASCADE,
// which takes River's tables with it.
//
// The guard is river_migration's existence, not a flag. A test's copy of the
// template already holds River's tables: the lane migrates the template with
// cmd/migrate up, River included. A rebuilt database holds none. The
// check-then-migrate window needs no lock: one test owns its database.
//
// migrate arrives as a parameter rather than an import. The layering permits the
// import — platform may depend on platform — but taking it would link River's
// runtime into every integration binary that wants nothing from it beyond a
// migrated schema. Passing the migrator keeps this the one spelling of the probe
// for every caller, the integration harness and the jobs suite alike, and leaves
// the dependency where it is actually used.
func EnsureRiverSchema(ctx context.Context, ownerPool *pgxpool.Pool, migrate func(context.Context, *pgxpool.Pool) (int, error)) error {
	var present bool
	if err := ownerPool.QueryRow(ctx,
		`SELECT to_regclass('public.river_migration') IS NOT NULL`).Scan(&present); err != nil {
		return fmt.Errorf("checking the river schema: %w", err)
	}
	if present {
		return nil
	}
	if _, err := migrate(ctx, ownerPool); err != nil {
		return fmt.Errorf("applying the river schema: %w", err)
	}
	return nil
}

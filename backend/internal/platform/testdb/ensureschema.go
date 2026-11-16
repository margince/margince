// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

// Package testdb gives each integration test a database of its own: a copy of
// the migrated template (New, For), with pools sized to the lane's budget.
package testdb

import (
	"context"
	"sync"
	"sync/atomic"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/dbmigrate"
	"github.com/margince/margince/backend/internal/platform/sweepreach"
	"github.com/margince/margince/backend/migrations"
)

var (
	migrateOnce sync.Once
	migrateErr  error

	// schemaReady reports that EnsureSchema has migrated this process's
	// database. Pool refuses a pool on a database that is not a test's copy
	// until then.
	schemaReady atomic.Bool
)

// EnsureSchema drops and rebuilds the schema of a database that is not a
// test's copy, once per process. A copy is at head already. The bench targets
// (InPlaceEnv) and a suite that makes its own database rely on the rebuild.
func EnsureSchema(ctx context.Context, owner *pgx.Conn) error {
	// A suite merges and erases on a test database without booting; one that
	// needs the declared tables installs them itself (integration.Setup).
	sweepreach.InstallForTestDatabase()
	if isCopy(owner.Config().Database) {
		return nil
	}
	migrateOnce.Do(func() {
		if err := rebuildSchema(ctx, owner); err != nil {
			migrateErr = err
			return
		}
		// The drift lane's clock shadow is per database and reaches connections
		// opened after it, so it goes on before any pool is released.
		if err := installClockShadow(ctx, owner); err != nil {
			migrateErr = err
			return
		}
		schemaReady.Store(true)
	})
	return migrateErr
}

// rebuildSchema drops the schema and applies every embedded migration.
func rebuildSchema(ctx context.Context, owner *pgx.Conn) error {
	if err := dropPublicSchema(ctx, owner); err != nil {
		return err
	}
	core, err := migrations.Core()
	if err != nil {
		return err
	}
	custom, err := migrations.Custom()
	if err != nil {
		return err
	}
	_, err = dbmigrate.Up(ctx, owner, core, custom)
	return err
}

// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package dbsettings declares database boot settings.
// It sits apart from package database because platform/config already
// imports that package.
package dbsettings

import (
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/platform/config"
	"github.com/margince/margince/backend/internal/platform/database"
)

// DSN is optional to the loader: migrate may take OwnerDSN instead. The api
// and the worker require it through Require.
var DSN = config.Define(config.Spec[string]{
	Key: "dsn", Env: "MARGINCE_DSN", Flag: "dsn", Type: config.String, Secret: true,
	Roles: []config.Role{config.RoleAPI, config.RoleWorker, config.RoleMigrate},
	Doc:   "Postgres DSN of the runtime app role; migrate falls back to it when MARGINCE_OWNER_DSN is unset.",
})

// OwnerDSN is the role that runs DDL. The app role in DSN holds no DDL
// rights, so migrate prefers this one.
var OwnerDSN = config.Define(config.Spec[string]{
	Key: "owner_dsn", Env: "MARGINCE_OWNER_DSN", Type: config.String, Secret: true,
	Roles: []config.Role{config.RoleMigrate},
	Doc:   "Postgres DSN of the owner role, which migrate applies the schema with.",
})

// ReadConcurrency bounds expensive reads independently of the connection count.
var ReadConcurrency = config.Define(config.Spec[int]{
	Key: "database.read_concurrency", Env: "MARGINCE_DATABASE_READ_CONCURRENCY", Type: config.Int,
	Default: config.Some(database.DefaultReadConcurrency), Min: config.Some(1),
	Roles: []config.Role{config.RoleAPI, config.RoleWorker},
	Doc:   "Limit on concurrent Worklist and ranked-search reads per process, capped at half the pool with at least one slot. Restart to apply.",
})

// ConfigureReadAdmission applies the boot setting before a pool serves traffic.
func ConfigureReadAdmission(pool *pgxpool.Pool, c *config.Config) error {
	return database.ConfigureReadAdmission(pool, config.Val(c, ReadConcurrency))
}

// Settings are this package's boot settings.
func Settings() []config.Any { return []config.Any{DSN, OwnerDSN, ReadConcurrency} }

// Require is the fault of a program that cannot start without DSN, worded as
// the loader words a missing value.
func Require(c *config.Config) error {
	if config.Val(c, DSN) == "" {
		return fmt.Errorf("%s: required", DSN.Env())
	}
	return nil
}

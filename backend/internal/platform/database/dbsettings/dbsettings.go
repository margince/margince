// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package dbsettings declares database boot settings.
// It sits apart from package database because platform/config already
// imports that package.
package dbsettings

import (
	"fmt"
	"time"

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

// Connection ages bound how long a backend retains planner memory.
var (
	ConnLifetime = config.Define(config.Spec[time.Duration]{
		Key: "database.connection_lifetime", Env: "MARGINCE_DATABASE_CONNECTION_LIFETIME", Type: config.Duration,
		Default: config.Some(database.DefaultConnLifetime), Min: config.Some(time.Minute),
		Roles: []config.Role{config.RoleAPI, config.RoleWorker},
		Doc: "Time a connection in the API or worker app pool can stay open, plus up to `20%` more. " +
			"The pool checks the age when a query ends and on each health check. Closing a direct Postgres connection returns memory kept after planning. " +
			"A `pool_max_conn_lifetime` value in the DSN wins. Restart to apply.",
	})
	ConnIdleTime = config.Define(config.Spec[time.Duration]{
		Key: "database.connection_idle_time", Env: "MARGINCE_DATABASE_CONNECTION_IDLE_TIME", Type: config.Duration,
		Default: config.Some(database.DefaultConnIdleTime), Min: config.Some(time.Minute),
		Roles: []config.Role{config.RoleAPI, config.RoleWorker},
		Doc: "Time a connection above `pool_min_conns` can stay open while not in use. Applies to API and worker app pools. " +
			"A `pool_max_conn_idle_time` value in the DSN wins. Restart to apply.",
	})
)

// PoolDSN is dsn with the connection lifetime and idle time named in it, so
// NewPool reads them. A value the DSN already names stays.
func PoolDSN(dsn string, c *config.Config) string {
	dsn = database.WithDSNParam(dsn, "pool_max_conn_lifetime", config.Val(c, ConnLifetime).String())
	return database.WithDSNParam(dsn, "pool_max_conn_idle_time", config.Val(c, ConnIdleTime).String())
}

// ConfigureReadAdmission applies the boot setting before a pool serves traffic.
func ConfigureReadAdmission(pool *pgxpool.Pool, c *config.Config) error {
	return database.ConfigureReadAdmission(pool, config.Val(c, ReadConcurrency))
}

// Settings are this package's boot settings.
func Settings() []config.Any {
	return []config.Any{DSN, OwnerDSN, ReadConcurrency, ConnLifetime, ConnIdleTime}
}

// Require is the fault of a program that cannot start without DSN, worded as
// the loader words a missing value.
func Require(c *config.Config) error {
	if config.Val(c, DSN) == "" {
		return fmt.Errorf("%s: required", DSN.Env())
	}
	return nil
}

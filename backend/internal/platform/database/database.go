// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package database is the shared Postgres platform layer: the configured
// connection pool and the transaction seam every store uses. It is the ONE
// place a transaction is opened for domain work, which is what makes "a
// module's row, its audit row and its outbox row commit together" a property
// one file holds rather than a habit every store repeats.
package database

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// PoolConfig is what a pool of this product IS: the operational limits, the
// runtime parameters and the per-connection registration every pool needs,
// settled from dsn and applied to nothing yet.
//
// It is separate from NewPool so that WHEN a pool dials is the caller's choice
// and WHAT it is is not. NewPool opens eagerly and pings, which is right for a
// process whose next act is to serve traffic: a bad DSN should fail the boot,
// not the first request. A caller that must not dial during construction — a
// test asserting what a CLOSED pool does, whose DSN points at nothing on
// purpose — takes the config and opens it itself, and still gets the ID type
// registration and the JIT setting rather than pgxpool's bare defaults.
//
// Each limit is a fallback, not a mandate: an operator who sized the pool in
// the DSN (pool_max_conns=…) knows their Postgres better than a hardcoded 16
// does, so a DSN-provided value always wins.
func PoolConfig(dsn string) (*pgxpool.Config, error) {
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("pg: parsing DSN: %w", err)
	}
	// ParseConfig already applied any pool_* DSN parameters; only fill
	// the ones the DSN left unset.
	if !strings.Contains(dsn, "pool_max_conns") {
		cfg.MaxConns = 16
	}
	if !strings.Contains(dsn, "pool_min_conns") {
		cfg.MinConns = 2
	}
	if !strings.Contains(dsn, "pool_max_conn_lifetime") {
		cfg.MaxConnLifetime = 30 * time.Minute
	}
	if !strings.Contains(dsn, "pool_max_conn_idle_time") {
		cfg.MaxConnIdleTime = 5 * time.Minute
	}
	if !strings.Contains(dsn, "pool_health_check_period") {
		cfg.HealthCheckPeriod = time.Minute
	}
	// JIT is off for this workload, and the row-scope predicates are why.
	// Every list, search and timeline query composes the caller's
	// visibility clause — nested EXISTS over person, organization, deal,
	// activity_link and record_grant — which inflates the plan's ESTIMATED
	// cost past jit_above_cost while the query itself stays an indexed
	// OLTP read. Postgres then spends longer in LLVM than in the query:
	// the /search union measured 12ms of work behind 475ms of JIT
	// generation, inlining, optimization and emission.
	//
	// What crosses the threshold is the row-scope TIER, not data volume,
	// so a rep pays it on a query an unbounded admin runs for free. JIT
	// earns its keep on long analytical scans; this product runs none on
	// the request path. A DSN that names jit itself still wins.
	if !strings.Contains(dsn, "jit") {
		if cfg.ConnConfig.RuntimeParams == nil {
			cfg.ConnConfig.RuntimeParams = map[string]string{}
		}
		cfg.ConnConfig.RuntimeParams["jit"] = "off"
	}
	// Typed entity ids ride uuid/uuid[] on every connection.
	cfg.AfterConnect = func(_ context.Context, conn *pgx.Conn) error {
		RegisterIDTypes(conn)
		return nil
	}
	return cfg, nil
}

// NewPool opens a pgxpool on PoolConfig's terms and proves it can reach the
// database before handing it back. A defaultless pool under load exhausts
// Postgres connections and hides slow queries; a pool that cannot connect at
// all is a boot failure, and saying so here is cheaper than discovering it on
// the first query.
func NewPool(ctx context.Context, dsn string) (*pgxpool.Pool, error) {
	cfg, err := PoolConfig(dsn)
	if err != nil {
		return nil, err
	}
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("pg: opening pool: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("pg: ping: %w", err)
	}
	return pool, nil
}

// ErrNoWorkspace means a domain query was attempted outside a workspace
// context — a programming error, surfaced before any SQL runs.
var ErrNoWorkspace = errors.New("pg: no workspace bound to context")

// WithWorkspaceTx runs fn inside ONE transaction, and refuses before any SQL
// runs if the context carries no workspace.
//
// It is the seam every domain read and write goes through: one transaction
// boundary per unit of work, so a module's row, its audit row and its outbox
// row commit together, and one place to audit what addresses the database.
// There is no raw-pool path for domain data — scripts/check-rls-store-path.sh
// holds that.
//
// The workspace itself scopes nothing in SQL. An installation holds exactly one
// (ADR-0061), no table carries workspace_id and no policy reads one, so what
// this checks is that a domain call was reached through a request that resolved
// its tenant at all — a programming-error check, not an isolation boundary.
func WithWorkspaceTx(ctx context.Context, pool *pgxpool.Pool, fn func(pgx.Tx) error) error {
	if _, ok := principal.WorkspaceID(ctx); !ok {
		return ErrNoWorkspace
	}

	return runTx(ctx, pool, fn)
}

// WithInfraTx runs fn in a transaction for the narrow infra paths that run
// outside any one tenant's request — workspace bootstrap, session lookup by
// token hash, the outbox relay. It differs from WithWorkspaceTx in exactly one
// way: it does not require a workspace on the context, which is why a path that
// HAS one must not reach for it.
func WithInfraTx(ctx context.Context, pool *pgxpool.Pool, fn func(pgx.Tx) error) error {
	return runTx(ctx, pool, fn)
}

// runTx is the one spelling of this package's transaction discipline — begin,
// roll back on every path that is not a commit, commit once — so the seams
// above cannot drift from each other while both exist.
//
// Held by: TestTheDatabasePackageOpensATransactionInOneFunction (backend/gates/transactionopeners_test.go)
func runTx(ctx context.Context, pool *pgxpool.Pool, fn func(pgx.Tx) error) error {
	return runTxWith(ctx, pool, pgx.TxOptions{}, fn)
}

// runTxWith is runTx with transaction options — the isolation level, for a read
// whose answer is composed from several statements and must therefore see one
// snapshot. Set at BEGIN because Postgres refuses the level once any query in
// the transaction has taken one.
func runTxWith(ctx context.Context, pool *pgxpool.Pool, opts pgx.TxOptions, fn func(pgx.Tx) error) error {
	tx, err := pool.BeginTx(ctx, opts)
	if err != nil {
		return fmt.Errorf("pg: begin: %w", err)
	}
	// The deferred rollback only matters on the error path; after a
	// successful Commit it answers ErrTxClosed by design, and on the error
	// path the fn/commit error is the one the caller must see.
	//craft:ignore swallowed-errors rollback after commit is a designed no-op; on the error path the fn error supersedes it
	defer func() { _ = tx.Rollback(ctx) }()

	if err := fn(tx); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

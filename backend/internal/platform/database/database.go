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
	"strconv"
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
	// visibility clause — nested EXISTS over contact, company, deal,
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
	setRuntimeParam(cfg, dsn, "jit", "off")
	// Nothing else bounds how long one statement may hold a connection.
	// http.Server's WriteTimeout ends the RESPONSE without cancelling the
	// handler, so a query that has stopped making progress keeps its slot in a
	// pool of sixteen until Postgres or the operator ends it — and the routes
	// that cost the most slots per request are exactly the ones a user waits
	// on. The ceilings and their sizing live beside CallerPredicateBudget,
	// which is the tighter ceiling the caller-written predicates ask for.
	setDurationParam(cfg, dsn, "statement_timeout", StatementCeiling)
	setDurationParam(cfg, dsn, "idle_in_transaction_session_timeout", IdleTransactionCeiling)
	// Typed entity ids ride uuid/uuid[] on every connection.
	cfg.AfterConnect = func(_ context.Context, conn *pgx.Conn) error {
		RegisterIDTypes(conn)
		return nil
	}
	return cfg, nil
}

// setRuntimeParam fills in a Postgres runtime parameter the DSN left unset.
// Same rule as every pool limit above: an operator who named the parameter in
// the DSN meant it, so a DSN-provided value always wins — including a zero,
// which Postgres reads as no bound at all and is how a role that must not be
// bounded says so.
func setRuntimeParam(cfg *pgxpool.Config, dsn, name, value string) {
	if strings.Contains(dsn, name) {
		return
	}
	if cfg.ConnConfig.RuntimeParams == nil {
		cfg.ConnConfig.RuntimeParams = map[string]string{}
	}
	cfg.ConnConfig.RuntimeParams[name] = value
}

// setDurationParam is setRuntimeParam in the unit Postgres reads a bare
// duration in. The conversion lives here rather than at each ceiling, because
// one off by a factor of a thousand either never fires or fires on everything,
// and the config it was written into looks the same either way.
func setDurationParam(cfg *pgxpool.Config, dsn, name string, value time.Duration) {
	setRuntimeParam(cfg, dsn, name, strconv.FormatInt(value.Milliseconds(), 10))
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
	// A read composed from many statements binds ONE snapshot to the context
	// and every store beneath it lands here; joining that snapshot is what
	// makes the composition cost one transaction instead of forty.
	if tx, joined := snapshotOf(ctx); joined {
		return fn(tx)
	}
	return runTx(ctx, pool, fn)
}

// snapshotKey is the context key the ambient read snapshot travels under. A
// package-private zero-size type, so nothing outside this file can reach the
// transaction or plant one.
type snapshotKey struct{}

// snapshotOf answers the snapshot bound to ctx, if any.
//
// no struct behind it to return instead.
//
//nolint:ireturn // pgx.Tx IS the transaction in this driver — an interface with
func snapshotOf(ctx context.Context) (pgx.Tx, bool) {
	tx, ok := ctx.Value(snapshotKey{}).(pgx.Tx)
	return tx, ok
}

// WithWorkspaceSnapshot runs fn against ONE read-only snapshot, which every
// WithWorkspaceTx reached beneath it joins instead of opening its own.
//
// FOR A READ WHOSE ANSWER IS COMPOSED FROM MANY STATEMENTS, and the worklist is
// the largest in the product: it assembles six fixed lanes and sixteen optional
// ones, and before this each of those ~40 dependency calls paid BEGIN + SELECT
// + COMMIT of its own — ~120 sequential round trips, which is most of the
// 400 ms that surface answered in (margince#4912).
//
// THE CORRECTNESS ARGUMENT IS THE STRONGER ONE. Those lanes were reading in
// twenty-odd different snapshots, so two lanes could disagree inside a single
// answer — a deal that closed between lane 3 and lane 11 appeared in one and
// not the other, and nothing on the response said so. REPEATABLE READ is what
// makes one assembled day one instant.
//
// READ-ONLY IS LOAD-BEARING, not caution. The join is ambient: a store beneath
// this cannot see that its transaction is somebody else's and would otherwise
// commit a domain write as part of a page assembly, so that a failure three
// lanes later silently rolls the write back. Postgres refusing the write is the
// loud version of that, and a write path that genuinely belongs under a
// composed read takes Detached below and says why.
//
// Nesting JOINS rather than opening a second transaction, so a composed read
// that calls another composed read still costs one.
func WithWorkspaceSnapshot(ctx context.Context, pool *pgxpool.Pool, fn func(context.Context) error) error {
	if _, ok := principal.WorkspaceID(ctx); !ok {
		return ErrNoWorkspace
	}
	if _, joined := snapshotOf(ctx); joined {
		return fn(ctx)
	}
	opts := pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly}
	return runTxWith(ctx, pool, opts, func(tx pgx.Tx) error {
		return fn(context.WithValue(ctx, snapshotKey{}, tx))
	})
}

// Detached returns ctx with no ambient snapshot on it, so a call made with it
// opens its own transaction and commits on its own terms.
//
// For the write a composed read legitimately makes. It is deliberately awkward
// to reach for: a write that joins a read-only snapshot is refused by Postgres,
// and a caller reaching here is answering that refusal, so the reason belongs
// at the call site.
func Detached(ctx context.Context) context.Context {
	if _, joined := snapshotOf(ctx); !joined {
		return ctx
	}
	return context.WithValue(ctx, snapshotKey{}, nil)
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

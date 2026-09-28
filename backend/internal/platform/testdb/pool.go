// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package testdb

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"strconv"
	"sync"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/platform/database"
)

// This file gives the lane one pool per DSN per test PROCESS, rather than one
// per test. The connections are the cost: a package's tests run sequentially
// against one clone database, and each test was opening a pool that eagerly
// dialled MinConns backends, using them once and closing them — while the lane
// runs several packages at once against ONE server. How many connections that
// server has is not a fact this file may assume: it is the lane's arithmetic,
// declared in scripts/lib-testdb.sh and provisioned in
// docker-compose.dev.yml, and this pool's part in it is the ceiling it is
// handed (see PoolMaxConnsEnv). Naming a figure here instead puts a second copy
// of it in the file a reader opens first.

// ErrSchemaNotReady is returned when a pool is asked for before EnsureSchema has
// migrated the database.
//
// It is an error rather than a comment because the blast radius is process-wide,
// and for two reasons that are demonstrable rather than plausible. A live session
// from an early pool holds locks on the relations `DROP SCHEMA public CASCADE`
// must take ACCESS EXCLUSIVE on, so the migration BLOCKS and the package dies at
// its go-test timeout with nothing naming the cause. And any query in flight while
// the schema is demolished and rebuilt fails inside whichever test happens to be
// running, never the one that broke the ordering.
//
// Not because a pooled connection would hold stale plan descriptions: describe_exec
// keeps none. That was true of the mode this pool used to run and is the kind of
// reason worth deleting when it stops being one.
//
// Unrecoverable within the package either way, because the pool is memoized for
// the process: nothing after the mistake re-opens it.
var ErrSchemaNotReady = errors.New(
	"testdb: the shared pool was requested before EnsureSchema migrated the database — call Setup (or EnsureSchema) first; a session open across the migration's DROP SCHEMA blocks it, and the pool is memoized for the process so nothing later recovers")

var (
	poolsMu sync.Mutex
	pools   = map[string]*pgxpool.Pool{}
)

// testPoolParams are this pool's declared differences from the one cmd/api
// opens, and the only ones. Everything else — jit=off, the typed-id
// registration, the lifetime and health-check limits, the opening Ping that
// makes an unusable DSN a loud setup failure — comes from database.NewPool
// below, so the lane exercises the product's pool with a named delta rather than
// a second constructor that drifts from it.
//
//	pool_min_conns=0          nothing is dialled until a test asks. The old
//	                          per-test pools dialled MinConns eagerly at every
//	                          Setup, which is the cost this change removes.
//	pool_max_conns            the ceiling, and the ONE parameter here the lane
//	                          rather than this file decides — see PoolMaxConnsEnv.
//	                          "A shared pool cannot exceed what each per-test pool
//	                          already had" is true per package and is the wrong
//	                          quantity: the lane's peak is that ceiling times
//	                          INTEGRATION_JOBS, so only the lane can choose it.
//	pool_max_conn_idle_time   a package that spiked hands its connections back
//	                          promptly instead of holding its high-water mark
//	                          for the rest of the run.
//	default_query_exec_mode   the only mode that is both cache-free and
//	                          server-typed, which is what THIS pool's connections
//	                          need in order to outlive a schema change — see the
//	                          note on Pool.
//
// The first two change how many connections exist; the third is the only one that
// changes how a query is executed, and so the only place this pool's behaviour
// parts from the api's.
var testPoolParams = map[string]string{
	"pool_min_conns":          "0",
	"pool_max_conn_idle_time": "30s",
	"default_query_exec_mode": "describe_exec",
}

// PoolMaxConnsEnv is how the parallel lane hands this process the per-pool
// ceiling it budgeted for.
//
// The lane runs INTEGRATION_JOBS packages against ONE server, so the demand is a
// product and the ceiling is the lane's to choose — a package cannot know how
// many of its siblings are live. Left unset (the one-package lane, a suite run
// by hand) the pool keeps database.NewPool's own 16, because one package
// oversubscribes nothing.
//
// It arrives as an environment variable rather than in the DSN, and that is not
// a style choice: cmd/migrate and every fixture that dials a bare pgx.Conn parse
// these same DSNs with pgx.ParseConfig, which does NOT strip pool_* keys — it
// forwards an unrecognised one to the server as a startup parameter, and the
// connection dies with `FATAL: unrecognized configuration parameter
// "pool_max_conns"`. Only pgxpool.ParseConfig knows those keys.
const PoolMaxConnsEnv = "MARGINCE_TEST_POOL_MAX_CONNS"

// poolParams is testPoolParams plus the lane's ceiling when the lane declared
// one. A malformed or non-positive value is an error rather than a silent
// fallback: a ceiling that fails to apply leaves the lane's budget describing a
// limit nothing enforces, which reads exactly like a bounded lane.
func poolParams() (map[string]string, error) {
	params := make(map[string]string, len(testPoolParams)+1)
	for k, v := range testPoolParams {
		params[k] = v
	}
	raw, ok := os.LookupEnv(PoolMaxConnsEnv)
	if !ok || raw == "" {
		return params, nil
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n <= 0 {
		return nil, fmt.Errorf("testdb: %s=%q is not a positive connection count — the lane sets it to the per-pool ceiling its connection budget was sized for, and a pool that ignores it makes that budget fiction", PoolMaxConnsEnv, raw)
	}
	params["pool_max_conns"] = strconv.Itoa(n)
	return params, nil
}

// Pool returns the process-wide pool for dsn, opening it on first use. Keyed by
// DSN, so the app-role and owner-role pools are distinct pools with the same
// lifetime.
//
// Deliberately NOT closed per test: it is closed when the test process exits,
// which is what makes it shared. Callers must not close it — a closed shared pool
// fails every later test in the package with a use-after-close that names nothing.
//
// What sharing costs is the ability to cache a statement, and describe_exec is
// how that is paid. It asks the server to describe the statement on every
// execution, so no statement cache spans a schema change.
//
// That is narrower than "nothing is cached", and the difference is the next trap.
// The per-connection pgtype.Map still memoizes encode plans, keyed by OID and
// invalidated only by a type registration — never by DDL. It is harmless today
// only because nothing here registers a schema-derived type: RegisterIDTypes maps
// to the fixed system uuid OIDs, and no caller uses conn.LoadType or
// RegisterType, so an extension's type falls to the same format-independent plan
// whatever its OID is. The first caller to LoadType("vector") or register a
// composite reopens this, and this mode alone will not cover it.
//
// This is also a property of THIS pool, not of the lane. Every other connection
// the suites open — the owner pgx.Conn each fixture dials, SchemaPool, and every
// pool a suite opens directly — still runs pgx's default cache_statement. Those
// close with their test, so a stale plan costs one test rather than the process,
// which is why they are left alone.
//
// pgx offers four other modes and none of them will do here. The two that cache —
// cache_statement, which is production's default, and cache_describe — both
// document that the first execution after a schema change may fail; a cached
// NAMED PLAN fails as SQLSTATE 0A000 "cached plan must not change result type",
// and a cached PARAMETER OID fails as XX000 "cache lookup failed for type N" once
// that type is recreated. The other two, exec and simple_protocol, cache nothing
// and cost no extra round trip, but they infer parameter types from the Go values
// instead of asking the server, which cannot encode the typed-id slices the record
// stores bind through ANY($n) — pool_integration_test.go pins that bind for this
// reason. describe_exec is the only mode that is both cache-free and server-typed.
//
// Only the XX000 form has been observed here, and it is the one this package
// gates: execmode_integration_test.go reproduces it without pgvector, so the
// choice above is enforced rather than argued. The mechanism it stands in for is
// real — search's embedding upsert binds $5::vector, so the server types that
// parameter as pgvector's, and a suite that drops the schema and remigrates gives
// the type a new OID under a live pooled connection. The 0A000 form is pgx's
// documented behaviour for a named plan; whether production meets it is a separate
// question with its own issue.
//
// The delta from production is narrower than swapping a mode sounds. Parameter
// OIDs and result formats still come from the server, so the encode and decode
// paths the lane exercises are the ones cmd/api uses; what differs is one extra
// round trip per statement that takes arguments, and the absent cache. That cost
// is real on a lane bound by round trips — 244s against the 190s the unsound
// cache_describe managed, same sitting, Postgres restarted before each run.
// Clearing the caches at each reset instead of forgoing them measured no cheaper:
// closing the connections cost 254s, and clearing them in place cost 215s but
// acquires every idle connection at each reset, which perturbs who gets a
// connection next.
//
// Two things this does not survive, neither of which applies: a connection pooler
// that switches the underlying connection between the two round trips, and a
// caller that queries before EnsureSchema has run — see ErrSchemaNotReady.
func Pool(ctx context.Context, dsn string) (*pgxpool.Pool, error) {
	if !schemaReady.Load() {
		return nil, ErrSchemaNotReady
	}
	poolsMu.Lock()
	defer poolsMu.Unlock()
	if pool, ok := pools[dsn]; ok {
		return pool, nil
	}
	tuned, err := withTestPoolParams(dsn)
	if err != nil {
		return nil, err
	}
	pool, err := database.NewPool(ctx, tuned)
	if err != nil {
		return nil, fmt.Errorf("opening the shared test pool: %w", err)
	}
	pools[dsn] = pool
	return pool, nil
}

// OwnPool opens a pool the CALLER owns and closes, tuned by the same lane
// ceiling the shared one takes, and dials nothing until the caller uses it.
//
// Most suites want Pool: one pool per DSN per process, memoized, which is the
// cheapest thing that works. A few genuinely cannot — a suite that asserts what
// happens when a pool is closed, one that needs a DSN of its own with extra
// parameters, one that hands a pool to a component that will close it — and
// before this existed those reached for database.NewPool directly.
//
// That is the hole this closes. database.NewPool's fallback is MaxConns=16 with
// MinConns=2 dialled eagerly, and the lane's budget is sized on the ceiling it
// hands out in PoolMaxConnsEnv — so one such suite could hold 8 (owner) + 8
// (app) + 16 (its own) against a declared per-package allowance of 24 while the
// lane's own arithmetic read green. The budget was a measured high-water mark
// rather than a ceiling, and this is what makes it the second thing.
//
// It is NOT memoized, which is the whole reason a caller asks for it: two calls
// answer two pools, and closing one does not affect the other. The caller closes
// it — nothing here will.
//
// It does NOT carry the shared pool's ErrSchemaNotReady check, and that is a
// decision rather than an omission. The check is a proxy for "this process has
// called EnsureSchema", which for a MEMOIZED pool is the right question: it
// outlives every test, so one opened beforehand is still open across the
// migration's DROP SCHEMA and blocks it. An owned pool's lifetime is its
// test's. And the proxy is wrong here in the direction that matters: a suite
// running in a package the lane already migrated has never called EnsureSchema
// itself, so the flag is false while the schema is perfectly ready — refusing
// there would refuse working suites for a hazard they do not have.
func OwnPool(ctx context.Context, dsn string) (*pgxpool.Pool, error) {
	tuned, err := withTestPoolParams(dsn)
	if err != nil {
		return nil, err
	}
	// The CONFIG, not the pool: nothing is dialled until a test asks. A suite
	// that owns its pool rather than sharing one is often owning it precisely
	// because it asserts what happens when the pool is broken — a DSN pointing
	// at a closed port, so that Close and the failure modes around it have
	// something to be true of — and a constructor that pinged would refuse
	// those suites during setup, before they could assert anything at all.
	// Everything else a pool of this product is, it still gets: the ID type
	// registration, the JIT setting, the operational limits.
	cfg, err := database.PoolConfig(tuned)
	if err != nil {
		return nil, err
	}
	pool, err := OwnPoolFromConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("opening a caller-owned test pool: %w", err)
	}
	return pool, nil
}

// OwnPoolFromConfig opens a caller-owned pool from a config the caller built,
// with the lane's ceiling applied to it.
//
// For the suites that cannot express what they need in a DSN: a runtime
// parameter set per connection, an AfterConnect hook, a dial that has to be
// bounded. Before this they reached for pgxpool.NewWithConfig and got pgxpool's
// own default — outside the lane's arithmetic exactly like database.NewPool's
// fallback, and quieter about it.
//
// It overrides MaxConns and MinConns and NOTHING else. The rest of the config
// is the caller's, because the reason they built one is that they needed
// something this package has no opinion about; the two it takes are the two the
// lane's budget is stated in.
//
// A ceiling the lane did not declare leaves MaxConns as the caller set it,
// which is the same posture withTestPoolParams takes for a DSN that names one.
func OwnPoolFromConfig(ctx context.Context, cfg *pgxpool.Config) (*pgxpool.Pool, error) {
	params, err := poolParams()
	if err != nil {
		return nil, err
	}
	if ceiling, declared := params["pool_max_conns"]; declared {
		// ParseInt with an explicit bit size rather than Atoi: MaxConns is an
		// int32, and a conversion that could overflow is one the compiler
		// cannot see and a reader has to reason about.
		n, convErr := strconv.ParseInt(ceiling, 10, 32)
		if convErr != nil {
			return nil, fmt.Errorf("testdb: the lane's ceiling %q is not a connection count: %w", ceiling, convErr)
		}
		// The LOWER of the two, never the ceiling flatly. A caller that asked
		// for one connection is asking for something the budget does not
		// forbid, and raising it would break the suites most likely to have
		// asked — the contention tests, whose whole subject is what happens
		// when the only connection is busy.
		if cfg.MaxConns <= 0 || int32(n) < cfg.MaxConns {
			cfg.MaxConns = int32(n)
		}
	}
	// Nothing is dialled until a test asks, which is the cost the shared pool
	// removed and the one an ad-hoc pool most often reintroduces.
	cfg.MinConns = 0
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("opening a caller-owned test pool from a config: %w", err)
	}
	return pool, nil
}

// withTestPoolParams adds testPoolParams to dsn without disturbing what is
// already there — a DSN that names one of them keeps its own value, matching
// database.NewPool's rule that whoever sized the pool explicitly wins.
func withTestPoolParams(dsn string) (string, error) {
	u, err := url.Parse(dsn)
	if err != nil {
		return "", fmt.Errorf("parsing the test DSN: %w", err)
	}
	params, err := poolParams()
	if err != nil {
		return "", err
	}
	q := u.Query()
	for k, v := range params {
		if !q.Has(k) {
			q.Set(k, v)
		}
	}
	u.RawQuery = q.Encode()
	return u.String(), nil
}

// redactDSN keeps the credentials in a test DSN out of a failure message: the
// lane's logs are CI artifacts, and the role name is what identifies the pool.
func redactDSN(dsn string) string {
	u, err := url.Parse(dsn)
	if err != nil {
		return "the test database"
	}
	if u.User != nil {
		u.User = url.User(u.User.Username())
	}
	return u.Redacted()
}

// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package database_test

import (
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"

	"github.com/margince/margince/backend/internal/platform/database"
)

// TestADSNParameterIsNamedInTheSpellingTheDSNUses: the two DSN forms carry
// parameters differently, and a parameter appended in the wrong one produces a
// DSN that still parses while the parameter is silently gone — a pool that
// looks configured and is not.
func TestADSNParameterIsNamedInTheSpellingTheDSNUses(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name string
		dsn  string
		want string
	}{
		{"a URL with no query string opens one", "postgres://h/db", "postgres://h/db?statement_timeout=5000"},
		{"a URL that already has one appends", "postgres://h/db?sslmode=disable", "postgres://h/db?sslmode=disable&statement_timeout=5000"},
		{"the postgresql scheme is the same form", "postgresql://h/db", "postgresql://h/db?statement_timeout=5000"},
		{"a keyword/value DSN gets it in front", "host=h dbname=db", "statement_timeout=5000 host=h dbname=db"},
		{"a DSN that already names it keeps its own", "host=h statement_timeout=1", "host=h statement_timeout=1"},
		{"a value is not a parameter name", "host=h password='statement_timeout'", "statement_timeout=5000 host=h password='statement_timeout'"},
		{"a quoted value may contain an assignment", "host=h password=' statement_timeout=1'", "statement_timeout=5000 host=h password=' statement_timeout=1'"},
		{"an encoded URL key still wins", "postgres://h/db?statement%5Ftimeout=1", "postgres://h/db?statement%5Ftimeout=1"},
		{"a malformed DSN is left for the pool to reject", "postgres://%/db", "postgres://%/db"},
		{"a longer name that starts with it is another parameter", "host=h statement_timeout_x=1", "statement_timeout=5000 host=h statement_timeout_x=1"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := database.WithDSNParam(tc.dsn, "statement_timeout", "5000"); got != tc.want {
				t.Errorf("WithDSNParam(%q) = %q, want %q", tc.dsn, got, tc.want)
			}
		})
	}
}

// TestAnAddedParameterLeavesEveryOtherValueAsItWas parses the result. A
// keyword/value DSN ending in an empty value or a backslash takes in what
// follows it, and the string still parses.
func TestAnAddedParameterLeavesEveryOtherValueAsItWas(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, dsn, password string
	}{
		{"an empty last value", "host=h user=app password=", ""},
		{"an empty last value with a trailing space", "host=h user=app password= ", ""},
		{"a quoted empty last value", "host=h user=app password=''", ""},
		{"a quoted value with spaces", "host=h password='a b=c'", "a b=c"},
		{"a trailing backslash", `host=h password=secret\`, "secret"},
		{"trailing whitespace", "host=h password=secret \t", "secret"},
		{"a URL with an empty password", "postgres://app:@h/db", ""},
		{"a URL with a query string", "postgres://app:secret@h/db?sslmode=disable", "secret"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			cfg, err := pgconn.ParseConfig(database.WithDSNParam(tc.dsn, "pool_max_conn_lifetime", "10m0s"))
			if err != nil {
				t.Fatalf("the DSN no longer parses: %v", err)
			}
			if cfg.Password != tc.password {
				t.Errorf("password became %q, want %q", cfg.Password, tc.password)
			}
			if got := cfg.RuntimeParams["pool_max_conn_lifetime"]; got != "10m0s" {
				t.Errorf("pool_max_conn_lifetime = %q, want 10m0s", got)
			}
		})
	}
}

// TestTheSchemaChangingRolesHaveNoCeilingAtAll: Postgres reads zero as no
// bound, and a lift that wrote any other number would cap DDL at whatever it
// happened to say.
func TestTheSchemaChangingRolesHaveNoCeilingAtAll(t *testing.T) {
	t.Parallel()
	lifted := database.WithoutRequestCeilings("host=h dbname=db")
	for _, name := range []string{"statement_timeout", "idle_in_transaction_session_timeout"} {
		if !strings.Contains(lifted, name+"=0") {
			t.Errorf("WithoutRequestCeilings left %s unlifted in %q; Postgres reads zero as no bound, and DDL needs that rather than a longer number", name, lifted)
		}
	}
}

// TestEveryCeilingAPoolSetsIsOneTheSchemaRolesCanLift derives its subject from
// PoolConfig rather than listing it: the two functions are two writers of one
// invariant — what bounds a connection — and a ceiling added to one and missed
// by the other does not fail here, it fails as a half-built index on somebody's
// installation. Anything named `*_timeout` counts, which is every shape a
// Postgres duration ceiling comes in.
func TestEveryCeilingAPoolSetsIsOneTheSchemaRolesCanLift(t *testing.T) {
	t.Parallel()
	const bare = "postgres://h/db"
	bounded, err := database.PoolConfig(bare)
	if err != nil {
		t.Fatalf("PoolConfig: %v", err)
	}
	lifted, err := database.PoolConfig(database.WithoutRequestCeilings(bare))
	if err != nil {
		t.Fatalf("PoolConfig on a lifted DSN: %v", err)
	}
	var ceilings int
	for name := range bounded.ConnConfig.RuntimeParams {
		if !strings.HasSuffix(name, "_timeout") {
			continue
		}
		ceilings++
		if got := lifted.ConnConfig.RuntimeParams[name]; got != "0" {
			t.Errorf("a pool sets %s but WithoutRequestCeilings leaves it at %q; the roles that run DDL lift what a pool sets, and this one they cannot", name, got)
		}
	}
	if ceilings == 0 {
		t.Fatal("found no ceiling on a pool at all — a pool of this product has always had some, so this read something smaller than it claims")
	}
}

// TestAPoolCarriesTheCeilingsItWasNotGiven: the ceilings are the pool's, not
// each caller's, so a pool opened by a role that never thought about them is
// still bounded. Read as milliseconds because that is the unit Postgres reads
// a bare integer in, and a timeout off by a factor of a thousand is a ceiling
// that either never fires or fires on everything.
func TestAPoolCarriesTheCeilingsItWasNotGiven(t *testing.T) {
	t.Parallel()
	cfg, err := database.PoolConfig("postgres://h/db")
	if err != nil {
		t.Fatalf("PoolConfig: %v", err)
	}
	for name, want := range map[string]int64{
		"statement_timeout":                   database.StatementCeiling.Milliseconds(),
		"idle_in_transaction_session_timeout": database.IdleTransactionCeiling.Milliseconds(),
	} {
		got := cfg.ConnConfig.RuntimeParams[name]
		if got != strconv.FormatInt(want, 10) {
			t.Errorf("a pool carries %s=%q, want %d milliseconds", name, got, want)
		}
	}
}

// TestAnOperatorsOwnCeilingWins is the same rule every pool limit above it
// follows. Without it the lift the schema-changing roles ask for is written
// into the DSN and then overwritten by the default it exists to remove.
func TestAnOperatorsOwnCeilingWins(t *testing.T) {
	t.Parallel()
	cfg, err := database.PoolConfig(database.WithoutRequestCeilings("postgres://h/db"))
	if err != nil {
		t.Fatalf("PoolConfig: %v", err)
	}
	for _, name := range []string{"statement_timeout", "idle_in_transaction_session_timeout"} {
		if got := cfg.ConnConfig.RuntimeParams[name]; got != "0" {
			t.Errorf("a pool overrode the DSN's %s with %q; the DSN is where a role says it runs DDL", name, got)
		}
	}
}

// TestAPoolRetiresItsConnectionsWithinMinutes: a backend keeps the memory it
// grew to plan a large query until its connection closes. The lifetime bounds
// that, and a DSN that names one of the two limits keeps its own value for
// that limit only.
func TestAPoolRetiresItsConnectionsWithinMinutes(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name             string
		dsn              string
		lifetime, jitter time.Duration
	}{
		{"a bare DSN gets both defaults", "postgres://h/db", 10 * time.Minute, 2 * time.Minute},
		{"a DSN lifetime wins and its jitter is a fifth of it", "postgres://h/db?pool_max_conn_lifetime=1h", time.Hour, 12 * time.Minute},
		{"a DSN jitter keeps the default lifetime", "postgres://h/db?pool_max_conn_lifetime_jitter=30s", 10 * time.Minute, 30 * time.Second},
		{"a keyword/value DSN names both", "host=h pool_max_conn_lifetime_jitter=1m pool_max_conn_lifetime=20m", 20 * time.Minute, time.Minute},
	} {
		cfg, err := database.PoolConfig(tc.dsn)
		if err != nil {
			t.Fatalf("%s: PoolConfig: %v", tc.name, err)
		}
		if cfg.ConnConfig.RuntimeParams["jit"] != "off" {
			t.Fatal("a pool setting suppressed the JIT ceiling")
		}
		if cfg.MaxConnLifetime != tc.lifetime || cfg.MaxConnLifetimeJitter != tc.jitter {
			t.Errorf("%s: lifetime %v jitter %v, want %v and %v",
				tc.name, cfg.MaxConnLifetime, cfg.MaxConnLifetimeJitter, tc.lifetime, tc.jitter)
		}
	}
}

func TestPoolLimitsIgnoreParameterNamesInValues(t *testing.T) {
	for _, dsn := range []string{
		"postgres://app:pool_max_conns@h/db",
		"host=h dbname=pool_min_conns password=pool_max_conn_idle_time",
	} {
		cfg, err := database.PoolConfig(dsn)
		if err != nil {
			t.Fatal(err)
		}
		if cfg.MaxConns != 16 || cfg.MinConns != 2 || cfg.MaxConnIdleTime != database.DefaultConnIdleTime {
			t.Fatalf("a value suppressed pool defaults: max=%d min=%d idle=%s", cfg.MaxConns, cfg.MinConns, cfg.MaxConnIdleTime)
		}
	}
	cfg, err := database.PoolConfig("postgres://h/db?pool%5Fmax_conns=4")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.MaxConns != 4 {
		t.Fatalf("encoded pool key overridden: got %d", cfg.MaxConns)
	}
	if _, err := database.PoolConfig(database.WithDSNParam("postgres://%/db", "pool_max_conns", "4")); err == nil {
		t.Fatal("a malformed DSN lost its parse error")
	}
}

func TestExplicitRuntimeParametersKeepTheirCaseInsensitiveNames(t *testing.T) {
	for _, dsn := range []string{"host=h JIT=on STATEMENT_TIMEOUT=0", "postgres://h/db?JIT=on&STATEMENT_TIMEOUT=0"} {
		cfg, err := database.PoolConfig(dsn)
		if err != nil {
			t.Fatal(err)
		}
		for name, want := range map[string]string{"JIT": "on", "STATEMENT_TIMEOUT": "0"} {
			if cfg.ConnConfig.RuntimeParams[name] != want {
				t.Fatalf("explicit %s was changed", name)
			}
			if _, duplicate := cfg.ConnConfig.RuntimeParams[strings.ToLower(name)]; duplicate {
				t.Fatalf("default duplicates explicit %s", name)
			}
		}
	}
}

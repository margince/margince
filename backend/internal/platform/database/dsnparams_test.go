// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package database_test

import (
	"strconv"
	"strings"
	"testing"

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
		{"a keyword/value DSN separates with a space", "host=h dbname=db", "host=h dbname=db statement_timeout=5000"},
		{"a DSN that already names it keeps its own", "host=h statement_timeout=1", "host=h statement_timeout=1"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := database.WithDSNParam(tc.dsn, "statement_timeout", "5000"); got != tc.want {
				t.Errorf("WithDSNParam(%q) = %q, want %q", tc.dsn, got, tc.want)
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

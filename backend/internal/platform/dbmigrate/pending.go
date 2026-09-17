// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package dbmigrate

// What a RUNNING process may ask about migration state, as opposed to what the
// migrator does about it.
//
// Up takes a *pgx.Conn, holds a cluster-wide advisory lock and creates the
// tracking table it is about to write. None of that belongs in a readiness
// probe: it runs on the serving pool, on every scrape, under the app role, and
// a probe that created a table would report a schema it had just invented.
// So this is read-only, lock-free, and says nothing about how to repair what
// it finds.

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
)

// Querier is the read surface Pending needs — satisfied by *pgxpool.Pool and
// by *pgx.Conn, so the serving pool answers without borrowing a connection of
// its own.
type Querier interface {
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// Shortfall is one namespace the database is behind this binary on.
type Shortfall struct {
	Namespace string
	// Missing are the versions this binary ships and the ledger does not
	// record, in the order the namespace holds them.
	Missing []string
	// Untracked says the tracking table does not exist at all: the namespace
	// was never applied here, rather than applied and behind. Missing then
	// holds every version the namespace ships, because none of them ran.
	//
	// Kept apart from Missing because the two are different operator actions —
	// one is "migrate", the other is usually "this database is not the one you
	// think it is" — and a message that could not tell them apart sends an
	// operator to the wrong one.
	Untracked bool
}

// Pending reports every namespace whose applied versions do not cover what
// this binary ships. An empty answer means the database is at or past head for
// all of them.
//
// PAST head is not a shortfall, and that asymmetry is deliberate. A database
// carrying a version this binary does not have is the far side of a rolling
// deploy — the old replica is still serving while the new schema is applied —
// and refusing there would take down the half of the fleet that is working.
// The direction that breaks is the other one: tables the code expects and the
// database does not have.
//
// It does not judge CONTENT. Up refuses to migrate past a version whose digest
// no longer matches, which is the right place for that answer: it can stop,
// and it holds the lock that makes the reading stable. A probe that reported a
// digest mismatch as not-ready would take a replica out of rotation for a
// condition no amount of waiting repairs.
func Pending(ctx context.Context, q Querier, namespaces ...Namespace) ([]Shortfall, error) {
	var short []Shortfall
	for _, ns := range namespaces {
		table := "schema_migrations_" + ns.Name
		// to_regclass reads the catalogs, which every role may read, and
		// answers NULL rather than raising for a relation that is not there —
		// so the absent case does not arrive as an error indistinguishable
		// from a lost connection.
		var exists *string
		if err := q.QueryRow(ctx, `SELECT to_regclass($1)::text`, "public."+table).Scan(&exists); err != nil {
			return nil, fmt.Errorf("pgmigrate: looking up %s: %w", table, err)
		}
		if exists == nil {
			short = append(short, Shortfall{
				Namespace: ns.Name,
				Missing:   versionsOf(ns),
				Untracked: true,
			})
			continue
		}
		applied, err := appliedVersionSet(ctx, q, table)
		if err != nil {
			return nil, err
		}
		var missing []string
		for _, m := range ns.Migrations {
			if !applied[m.Version] {
				missing = append(missing, m.Version)
			}
		}
		if len(missing) > 0 {
			short = append(short, Shortfall{Namespace: ns.Name, Missing: missing})
		}
	}
	return short, nil
}

func versionsOf(ns Namespace) []string {
	versions := make([]string, 0, len(ns.Migrations))
	for _, m := range ns.Migrations {
		versions = append(versions, m.Version)
	}
	return versions
}

// appliedVersionSet reads the ledger's versions alone. appliedVersions beside
// it reads the name and digest too and takes a *pgx.Conn; this asks the
// smaller question the probe has, of the pool it already holds.
func appliedVersionSet(ctx context.Context, q Querier, table string) (map[string]bool, error) {
	// The table name is not a parameter — an identifier never can be — so it
	// goes through Sanitize rather than into the string as it arrived.
	rows, err := q.Query(ctx, fmt.Sprintf(`SELECT version FROM %s`, pgx.Identifier{table}.Sanitize()))
	if err != nil {
		return nil, fmt.Errorf("pgmigrate: reading %s: %w", table, err)
	}
	defer rows.Close()
	applied := map[string]bool{}
	for rows.Next() {
		var version string
		if err := rows.Scan(&version); err != nil {
			return nil, fmt.Errorf("pgmigrate: reading %s: %w", table, err)
		}
		applied[version] = true
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("pgmigrate: reading %s: %w", table, err)
	}
	return applied, nil
}

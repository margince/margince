// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package dbmigrate

// The per-namespace ledger: the table that records what has been applied, and
// who may read it. Created on demand rather than by a migration, because a
// namespace can arrive with a unit installed long after any migration ran.

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
)

func trackingTable(ctx context.Context, conn *pgx.Conn, namespace string) (string, error) {
	// Digits are admitted because an extension namespace carries them
	// (`ext_foo_1`); the set stays exactly what an unquoted SQL identifier
	// holds, since the namespace is interpolated into the statement below and
	// cannot be a parameter.
	for i, r := range namespace {
		digit := r >= '0' && r <= '9'
		if (r < 'a' || r > 'z') && r != '_' && !digit {
			return "", fmt.Errorf("pgmigrate: namespace %q: want lower-case letters, digits and underscores", namespace)
		}
		if digit && i == 0 {
			return "", fmt.Errorf("pgmigrate: namespace %q: an identifier cannot start with a digit", namespace)
		}
	}
	if namespace == "" {
		return "", fmt.Errorf("pgmigrate: empty namespace: it keys the tracking table")
	}
	table := "schema_migrations_" + namespace
	_, err := conn.Exec(ctx, fmt.Sprintf(
		`CREATE TABLE IF NOT EXISTS %s (
			version        text PRIMARY KEY,
			name           text NOT NULL,
			applied_at     timestamptz NOT NULL DEFAULT now(),
			content_digest text
		)`, table))
	if err != nil {
		return "", fmt.Errorf("pgmigrate: creating %s: %w", table, err)
	}
	// The tracking tables are created by this function and never by a
	// migration, so a database that already has one predates the column and
	// CREATE TABLE IF NOT EXISTS will not add it. This does.
	//
	// NULLABLE, and it stays that way: a row written before the digest existed
	// records a version whose content nobody can now recover, and back-filling
	// it here would stamp a fingerprint over content this binary never applied
	// — which is precisely the divergence the column exists to expose. A NULL
	// means "unverifiable", and every reader must treat it as such.
	if _, err := conn.Exec(ctx, fmt.Sprintf(
		`ALTER TABLE %s ADD COLUMN IF NOT EXISTS content_digest text`, table)); err != nil {
		return "", fmt.Errorf("pgmigrate: adding %s.content_digest: %w", table, err)
	}
	if _, err := conn.Exec(ctx, fmt.Sprintf(grantTrackingTableToApp, table)); err != nil {
		return "", fmt.Errorf("pgmigrate: granting read on %s: %w", table, err)
	}
	return table, nil
}

// grantTrackingTableToApp lets the runtime role READ what has been applied.
//
// It is issued here rather than by a migration because these tables are
// created on demand and never by one: a static GRANT could not reach a unit
// installed after it ran. Nor does one need to reach the tables that already
// exist — Up calls this function for EVERY namespace on every run, whether or
// not that namespace has anything to apply, so an existing table is granted
// the next time the operator migrates, which is the same run a migration would
// have landed in.
//
// The runtime process needs it to answer whether it is serving against the
// schema it was built for — a binary that is ready while its tables do not yet
// exist publishes routes and jobs that fail with undefined-table errors, which
// is the ordinary rolling-deploy window. What it widens is read-only and small:
// which versions have been applied, the same information the boot log already
// prints. It is not tenant data, not a credential and not a capability, and it
// leaves AssertRuntimeRole's invariant — ownership, superuser, bypass —
// untouched.
//
// Conditional on the role existing, the same shape the core's own grant
// migrations and the River grant take, so a throwaway database that runs
// everything as the owner applies the same schema rather than failing here.
const grantTrackingTableToApp = `
DO $$
BEGIN
	IF EXISTS (SELECT FROM pg_roles WHERE rolname = 'margince_app') THEN
		EXECUTE 'GRANT SELECT ON public.%s TO margince_app';
	END IF;
END $$;`

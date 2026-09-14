// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package database

import "strings"

// WithDSNParam names a connection parameter in dsn, unless the DSN already
// names it — PoolConfig's rule that an operator who stated a value meant it,
// applied one level earlier so a ROLE can state one too.
//
// The two DSN spellings take different separators: the URL form carries
// parameters as a query string, the keyword/value form as space-separated
// pairs. Getting that wrong produces a DSN that parses and silently drops the
// parameter, which is the failure this exists to avoid at two call sites
// rather than one.
//
// Held by: TestADSNParameterIsNamedInTheSpellingTheDSNUses
// (backend/internal/platform/database/dsnparams_test.go)
func WithDSNParam(dsn, name, value string) string {
	if strings.Contains(dsn, name) {
		return dsn
	}
	param := name + "=" + value
	if strings.HasPrefix(dsn, "postgres://") || strings.HasPrefix(dsn, "postgresql://") {
		sep := "?"
		if strings.Contains(dsn, "?") {
			sep = "&"
		}
		return dsn + sep + param
	}
	return dsn + " " + param
}

// WithoutRequestCeilings lifts StatementCeiling and IdleTransactionCeiling from
// dsn, for the roles that run SCHEMA changes rather than serve requests: the
// migrator's River lane and the API's custom-field schema pool.
//
// An index build or a table rewrite legitimately outruns any request-shaped
// bound, and one cut half way leaves the schema short of head with nothing in
// the failure to say that a timeout — rather than the DDL — is what ended it.
// The advisory lock these paths queue on is a running statement too, so the
// statement ceiling would end the WAIT as readily as the work.
//
// Postgres reads zero as no bound at all, which is why the lift is a value and
// not a deletion: the parameter stays visible in the DSN, where an operator
// reading it can see that this role has no ceiling and that saying so was
// deliberate.
func WithoutRequestCeilings(dsn string) string {
	lifted := WithDSNParam(dsn, "statement_timeout", "0")
	return WithDSNParam(lifted, "idle_in_transaction_session_timeout", "0")
}

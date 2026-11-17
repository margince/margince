// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package database

import (
	"strings"

	"github.com/jackc/pgx/v5/pgconn"
)

// WithDSNParam preserves explicit operator settings in either DSN spelling.
func WithDSNParam(dsn, name, value string) string {
	if namesDSNParam(dsn, name) {
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
	// In front, because pgconn reads an empty last value or a trailing backslash
	// as swallowing whatever follows it.
	return param + " " + dsn
}

// pgconn leaves pool settings in RuntimeParams, before pgxpool consumes them.
func namesDSNParam(dsn, name string) bool {
	cfg, err := pgconn.ParseConfig(dsn)
	if err != nil {
		// Leave malformed DSNs unchanged so the pool opener reports their parse error.
		return true
	}
	for key := range cfg.RuntimeParams {
		if strings.EqualFold(key, name) {
			return true
		}
	}
	return false
}

// WithoutRequestCeilings lets schema changes outlive request timeouts.
// Explicit operator limits still win, including zero for unbounded work.
func WithoutRequestCeilings(dsn string) string {
	lifted := WithDSNParam(dsn, "statement_timeout", "0")
	return WithDSNParam(lifted, "idle_in_transaction_session_timeout", "0")
}

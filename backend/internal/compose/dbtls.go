// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/runtimeenv"
)

// AssertDatabaseTLS refuses a production boot whose Postgres DSN cannot
// guarantee TLS. It mirrors refuseUnlicensedProduction's shape exactly: a
// non-production installation is unheld, because MARGINCE_ENV is what names a
// deployment as one — the same distinction this tree already trusts for the
// license and the runtime role.
//
// "Cannot guarantee" is deliberate, not "did not request": sslmode's default
// (unset, which pgconn resolves to "prefer") and "allow" both let the first
// connection attempt negotiate TLS and silently RETRY IN THE CLEAR if that
// fails — a network path that starts encrypted and degrades to plaintext on
// the next reconnect, with nothing in this process ever noticing. Only
// require/verify-ca/verify-full remove that fallback, which is what
// tlsEnforced checks for directly off the parsed pgconn.Config rather than
// pattern-matching the DSN string — a DSN can name sslmode through a URL
// query, a keyword/value pair, or PGSSLMODE, and pgconn already resolves all
// three into the one struct this reads.
func AssertDatabaseTLS(dsn string, env runtimeenv.Environment) error {
	if env.IsNonProduction() {
		return nil
	}
	cfg, err := database.PoolConfig(dsn)
	if err != nil {
		return fmt.Errorf("compose: reading the Postgres DSN's TLS posture: %w", err)
	}
	if tlsEnforced(cfg) {
		return nil
	}
	return fmt.Errorf("this installation is production and its Postgres connection does not force TLS "+
		"(sslmode is unset, \"disable\", \"allow\" or \"prefer\" — each permits a plaintext connection): "+
		"add sslmode=require (or verify-ca / verify-full, to also check the server's certificate) to the DSN, "+
		"or, if this is a development or test installation, set %s=%s",
		runtimeenv.EnvVar, runtimeenv.Development)
}

// tlsEnforced reports whether every connection attempt pgconn would make for
// this config uses TLS — the primary attempt, and every fallback sslmode
// "prefer" or "allow" register for a plaintext retry when the primary one
// fails to negotiate TLS.
func tlsEnforced(cfg *pgxpool.Config) bool {
	if cfg.ConnConfig.TLSConfig == nil {
		return false
	}
	for _, fallback := range cfg.ConnConfig.Fallbacks {
		if fallback.TLSConfig == nil {
			return false
		}
	}
	return true
}

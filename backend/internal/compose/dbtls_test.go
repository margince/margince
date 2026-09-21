// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/runtimeenv"
)

// MARGINCE_ENV is fail-closed (runtimeenv.Parse("") is Production), so an
// installation that names no posture is held to this the same way it is held
// to a license.
func TestAssertDatabaseTLSRefusesAProductionBootWithoutEnforcedTLS(t *testing.T) {
	cases := map[string]string{
		"sslmode unset":   "postgres://app:pw@db.internal:5432/margince",
		"sslmode=disable": "postgres://app:pw@db.internal:5432/margince?sslmode=disable",
		"sslmode=allow":   "postgres://app:pw@db.internal:5432/margince?sslmode=allow",
		"sslmode=prefer":  "postgres://app:pw@db.internal:5432/margince?sslmode=prefer",
	}
	for name, dsn := range cases {
		t.Run(name, func(t *testing.T) {
			err := AssertDatabaseTLS(dsn, runtimeenv.Parse(""))
			if err == nil {
				t.Fatalf("AssertDatabaseTLS(%q) booted a production installation that permits a plaintext connection", dsn)
			}
			for _, want := range []string{"sslmode=require", runtimeenv.EnvVar, string(runtimeenv.Development)} {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("boot refusal %q does not name %q", err, want)
				}
			}
		})
	}
}

// require/verify-ca/verify-full all remove the plaintext fallback prefer and
// allow register — that is the one property this check holds, not a
// preference for any one of the three over the others.
func TestAssertDatabaseTLSAdmitsAnyModeThatRemovesThePlaintextFallback(t *testing.T) {
	for _, mode := range []string{"require", "verify-ca", "verify-full"} {
		t.Run(mode, func(t *testing.T) {
			dsn := "postgres://app:pw@db.internal:5432/margince?sslmode=" + mode
			if err := AssertDatabaseTLS(dsn, runtimeenv.Parse("")); err != nil {
				t.Fatalf("AssertDatabaseTLS(%q) = %v, want nil", dsn, err)
			}
		})
	}
}

// A non-production installation is unheld — the same MARGINCE_ENV escape
// hatch EnsureLicense already grants, and for the same reason: it is what
// lets make dev's own plaintext Postgres keep booting unedited.
func TestAssertDatabaseTLSAdmitsAnyModeInNonProduction(t *testing.T) {
	for _, env := range []runtimeenv.Environment{runtimeenv.Development, runtimeenv.Test} {
		t.Run(string(env), func(t *testing.T) {
			dsn := "postgres://app:pw@db.internal:5432/margince?sslmode=disable"
			if err := AssertDatabaseTLS(dsn, env); err != nil {
				t.Fatalf("AssertDatabaseTLS(%q, %q) = %v, want nil", dsn, env, err)
			}
		})
	}
}

func TestAssertDatabaseTLSNamesAnUnparseableDSN(t *testing.T) {
	err := AssertDatabaseTLS("not a dsn at all", runtimeenv.Parse(""))
	if err == nil {
		t.Fatal("AssertDatabaseTLS accepted an unparseable DSN")
	}
}

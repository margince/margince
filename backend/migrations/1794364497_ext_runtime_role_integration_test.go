// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package migrations_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/dbmigrate"
	"github.com/margince/margince/backend/migrations"
)

const extRuntimeVersion = "1794364497"

// The migration that grants ext_runtime its schema does not trust a role that
// already exists. Each case runs the migration's own text in a transaction
// that rolls back. It names a probe role in place of ext_runtime, because a
// role is cluster-wide and other test runs use the real one.
func TestTheExtRuntimeMigrationRefusesARoleThatHoldsTooMuch(t *testing.T) {
	ownerDSN, _ := dsns(t)
	conn := connect(t, ownerDSN)
	resetSchema(t, conn)
	ctx := context.Background()
	core, err := migrations.Core()
	if err != nil {
		t.Fatalf("loading core: %v", err)
	}
	upSQL := ""
	for i, m := range core.Migrations {
		if m.Version == extRuntimeVersion {
			upSQL = m.UpSQL
			if _, err := dbmigrate.Up(ctx, conn, dbmigrate.Namespace{Name: core.Name, Migrations: core.Migrations[:i]}); err != nil {
				t.Fatalf("up to the migration before %s: %v", extRuntimeVersion, err)
			}
		}
	}
	if upSQL == "" {
		t.Fatalf("no core migration carries version %s", extRuntimeVersion)
	}
	probe := fmt.Sprintf("ext_runtime_probe_%d", os.Getpid())
	migrator := fmt.Sprintf("ext_runtime_migrator_%d", os.Getpid())

	for _, c := range []struct {
		name, setup string
		// unprivileged runs the migration as a migrator that may not alter roles,
		// as on a deployed database.
		unprivileged bool
		want         string
	}{
		{name: "a new role", want: ""},
		{name: "a role that may log in, reset by a superuser migrator", setup: `CREATE ROLE ` + probe + ` LOGIN`, want: ""},
		{name: "a role that may log in", setup: `CREATE ROLE ` + probe + ` LOGIN NOINHERIT`, unprivileged: true, want: "holds an attribute it must not have"},
		{name: "a role that is a member of margince_app", setup: `CREATE ROLE ` + probe + ` NOLOGIN NOINHERIT; GRANT margince_app TO ` + probe, want: "is a member of margince_app"},
		{name: "a role that may read a core table", setup: `CREATE ROLE ` + probe + ` NOLOGIN NOINHERIT; GRANT SELECT ON public.contact TO ` + probe, want: "rights on tables outside schema ext: public.contact"},
	} {
		t.Run(c.name, func(t *testing.T) {
			err := pgx.BeginFunc(ctx, conn, func(tx pgx.Tx) error {
				if c.setup != "" {
					if _, err := tx.Exec(ctx, c.setup); err != nil {
						t.Fatalf("preparing the probe role: %v", err)
					}
				}
				if c.unprivileged {
					if _, err := tx.Exec(ctx, `CREATE ROLE `+migrator+` NOSUPERUSER NOCREATEROLE; SET LOCAL ROLE `+migrator); err != nil {
						t.Fatalf("becoming a migrator that may not alter roles: %v", err)
					}
				}
				if _, err := tx.Exec(ctx, strings.ReplaceAll(upSQL, "ext_runtime", probe)); err != nil {
					return err
				}
				// Rolled back whatever happened, so no probe role outlives the case.
				return errRolledBack
			})
			switch {
			case c.want == "" && !errors.Is(err, errRolledBack):
				t.Errorf("the migration refused a role it must accept: %v", err)
			case c.want != "" && (err == nil || !strings.Contains(err.Error(), c.want)):
				t.Errorf("the migration answered %v, want a refusal naming %q", err, c.want)
			}
		})
	}
}

var errRolledBack = errors.New("rolled back on purpose")

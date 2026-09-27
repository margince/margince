// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// The team_oversight backfill against the documents an installation really
// holds. The convergence arms in rbacseedparity prove the SEEDED matrix comes
// out right; these prove the two cases that matrix cannot show: a role an
// operator narrowed before upgrading, and a denial an operator set afterwards.

import (
	"context"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration/apptest"
	"github.com/margince/margince/backend/migrations"
)

const teamOversightVersion = "1790508883"

// runTeamOversightMigration executes one direction of the migration's own SQL.
func runTeamOversightMigration(ctx context.Context, t *testing.T, e *apptest.AppEnv, up bool) {
	t.Helper()
	core, err := migrations.Core()
	if err != nil {
		t.Fatalf("loading the core migrations: %v", err)
	}
	for _, migration := range core.Migrations {
		if migration.Version != teamOversightVersion {
			continue
		}
		sql := migration.DownSQL
		if up {
			sql = migration.UpSQL
		}
		tx, err := e.Owner.Begin(ctx)
		if err != nil {
			t.Fatalf("opening the migration transaction: %v", err)
		}
		if _, err := tx.Exec(ctx, sql); err != nil {
			t.Fatalf("running migration %s (up=%v): %v", teamOversightVersion, up, err)
		}
		if err := tx.Commit(ctx); err != nil {
			t.Fatalf("committing migration %s: %v", teamOversightVersion, err)
		}
		return
	}
	t.Fatalf("no core migration has version %s", teamOversightVersion)
}

// oversees reads whether a system role's stored document grants the read.
func oversees(ctx context.Context, t *testing.T, e *apptest.AppEnv, role string) bool {
	t.Helper()
	var read *bool
	if err := e.Owner.QueryRow(ctx,
		`SELECT (permissions -> 'objects' -> 'team_oversight' ->> 'read')::boolean
		   FROM role WHERE key = $1 AND is_system`, role).Scan(&read); err != nil {
		t.Fatalf("reading %s's team_oversight grant: %v", role, err)
	}
	if read == nil {
		t.Fatalf("%s carries no team_oversight key — the backfill missed it", role)
	}
	return *read
}

// The backfill never widens. A management role narrowed to team scope read only
// its own teams' weeks before the upgrade, so it gets no oversight; one left as
// seeded read every team's week, and keeps doing so through the grant.
func TestTheOversightBackfillNeverWidensANarrowedRole(t *testing.T) {
	for _, tc := range []struct {
		name     string
		narrowed bool
		want     bool
	}{
		{"as seeded", false, true},
		{"narrowed to team scope", true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := apptest.SetupApp(t)
			ctx := context.Background()
			bootstrapInstallation(t, e)
			rewindTo(ctx, t, e, []string{"team_oversight"})
			if tc.narrowed {
				if _, err := e.Owner.Exec(ctx,
					`UPDATE role SET permissions = jsonb_set(permissions, '{row_scope}', '"team"')
					  WHERE key = 'management' AND is_system`); err != nil {
					t.Fatalf("narrowing management: %v", err)
				}
			}

			runTeamOversightMigration(ctx, t, e, true)

			if got := oversees(ctx, t, e, "management"); got != tc.want {
				t.Errorf("management oversees every team = %v, want %v", got, tc.want)
			}
			if oversees(ctx, t, e, "read_only") {
				t.Error("read_only was granted oversight; its row scope reaches every record, not every verdict")
			}
		})
	}
}

// A rollback keeps an operator's denial. Removing the key on the way down would
// let the next upgrade read its absence as "never decided" and grant it again.
func TestTheOversightRollbackKeepsAnOperatorsDenial(t *testing.T) {
	e := apptest.SetupApp(t)
	ctx := context.Background()
	bootstrapInstallation(t, e)
	if _, err := e.Owner.Exec(ctx,
		`UPDATE role SET permissions = jsonb_set(permissions, '{objects,team_oversight}',
		        '{"create":false,"read":false,"update":false,"delete":false}'::jsonb, true)
		  WHERE key = 'management' AND is_system`); err != nil {
		t.Fatalf("denying management oversight: %v", err)
	}

	runTeamOversightMigration(ctx, t, e, false)
	runTeamOversightMigration(ctx, t, e, true)

	if oversees(ctx, t, e, "management") {
		t.Error("down then up turned the operator's denial on management back into a grant")
	}
}

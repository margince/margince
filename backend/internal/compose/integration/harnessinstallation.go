// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// The installation half of the harness: the settings rows that make the
// fixture an installation rather than a bare workspace row, and the seam the
// modules read them through.

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"
)

// seedInstallationIdentity writes the installation's own settings rows.
func seedInstallationIdentity(ctx context.Context, t *testing.T, owner *pgx.Conn) {
	t.Helper()
	// The installation's identity (ADR-0090/A135). This harness builds the
	// installation by raw SQL, so bootstrap never seeded these and 0191's
	// backfill ran before the workspace existed.
	//
	// All of them, because bootstrap writes all of them: name, currency,
	// language and zone are one act, and a fixture holding some of them is a
	// state no installation is ever in.
	//
	// These rows are the whole of it now. They used to have to match a copy on
	// the workspace row, and a suite whose two copies disagreed measured the
	// drift rather than the behaviour under test; the columns are gone, so
	// there is one place to seed and one place to read.
	if _, err := owner.Exec(ctx,
		`INSERT INTO setting (key, value) VALUES
			('installation.name', '"Authz"'::jsonb),
			('installation.base_currency', '"EUR"'::jsonb),
			('installation.base_language', '"en"'::jsonb),
			('installation.timezone', '"UTC"'::jsonb)
		 ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value`); err != nil {
		t.Fatal(err)
	}
}

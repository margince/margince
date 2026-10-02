// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package migrations_test

// An archived record releases the name it held.
//
// The committed catalog records the predicate, which is a statement about the
// schema's text. This is the other half: that Postgres actually lets the name be
// taken again, and still refuses a second LIVE row holding it. A predicate that
// released the name and also stopped guarding it would read correctly in the catalog
// and lose the uniqueness the index is for.

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
)

func TestAnArchivedRecordReleasesTheNameItHeld(t *testing.T) {
	ownerDSN, _ := dsns(t)
	conn := connect(t, ownerDSN)
	headSchema(t, conn)
	ctx := context.Background()

	for _, c := range []struct{ table, insert string }{
		{"pipeline", `INSERT INTO pipeline (name, archived_at) VALUES ($1, $2)`},
		{"team", `INSERT INTO team (name, archived_at) VALUES ($1, $2)`},
		{"offer_template", `INSERT INTO offer_template (name, layout, archived_at) VALUES ($1, '{}'::jsonb, $2)`},
	} {
		// Archived, then the same name again: the reader cannot see the first, so a
		// conflict here names a record they have no way to reach or rename.
		if _, err := conn.Exec(ctx, c.insert, "Reused", "2026-01-01T00:00:00Z"); err != nil {
			t.Fatalf("%s: seeding the archived row: %v", c.table, err)
		}
		if _, err := conn.Exec(ctx, c.insert, "Reused", nil); err != nil {
			t.Errorf("%s: a live row could not take the name an archived row held: %v", c.table, err)
			continue
		}
		// And the guard still guards: two LIVE rows with one name stay refused.
		_, err := conn.Exec(ctx, c.insert, "Reused", nil)
		var pgErr *pgconn.PgError
		if !errors.As(err, &pgErr) || pgErr.Code != "23505" {
			t.Errorf("%s: a SECOND live row took the same name (%v) — releasing the archived name "+
				"must not stop the index guarding the live ones", c.table, err)
		}
	}
}

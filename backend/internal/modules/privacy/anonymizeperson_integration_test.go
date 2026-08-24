// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package privacy

// The person/anonymize action against real rows.
//
// Two of the things this action does are invisible to a test that only asks
// whether the person row stopped naming the subject:
//
//   - the pending-counterparty ledger is keyed on the subject's ADDRESSES, and
//     those rows are deleted by the same action a few statements earlier. A
//     statement written against `person_email` here matches nothing and removes
//     nothing while reading exactly like one that works.
//   - a CUSTOM column is where an operator puts what the fixed schema has no
//     room for. Nulling every fixed column and leaving those anonymizes the
//     name and keeps whatever was written beside it.
//
// Both are asserted here because neither shows up in the table-set census in
// backend/personscrub_test.go: the first is an ordering property, and the
// second is two acts clearing one table to different depths.

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/gradionhq/margince/backend/internal/platform/testdb"
	"github.com/gradionhq/margince/backend/internal/shared/kernel/ids"
)

func TestAnonymizeClearsWhatIsKeyedOnTheSubjectsAddress(t *testing.T) {
	ownerDSN := os.Getenv("MARGINCE_TEST_DSN")
	if ownerDSN == "" {
		t.Fatal("MARGINCE_TEST_DSN not set — run `make db-up` (integration tests fail loudly, they never skip)")
	}
	ctx := context.Background()
	owner, err := pgx.Connect(ctx, ownerDSN)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := owner.Close(context.Background()); err != nil {
			t.Errorf("closing owner connection: %v", err)
		}
	})
	if err := testdb.EnsureSchema(ctx, owner); err != nil {
		t.Fatal(err)
	}

	tx, err := owner.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	// EVERYTHING below is inside this transaction, the DDL included: Postgres
	// rolls back an ALTER TABLE with everything else, and the lane database is
	// shared. A fixture that seeded outside it would leave a `cf_` column on
	// `person` for every test that ran afterwards.
	defer func() {
		if err := tx.Rollback(context.Background()); err != nil && !errors.Is(err, pgx.ErrTxClosed) {
			t.Errorf("rolling back: %v", err)
		}
	}()

	ws, user := ids.NewV7(), ids.NewV7()
	person := ids.New[ids.PersonKind]()
	const subjectEmail = "hedda@anon.test"
	mustExec(ctx, t, tx, `INSERT INTO workspace (id) VALUES ($1)`, ws)
	mustExec(ctx, t, tx,
		`INSERT INTO app_user (id, email, display_name) VALUES ($1, $2, 'Admin')`,
		user, "admin-"+user.String()+"@anon.test")
	mustExec(ctx, t, tx,
		`INSERT INTO person (id, full_name, source, captured_by)
		 VALUES ($1, 'Hedda Subject', 'manual', 'user:'||$2::text)`, person, user)
	mustExec(ctx, t, tx,
		`INSERT INTO person_email (person_id, email, source, captured_by)
		 VALUES ($1, $2, 'manual', 'user:'||$3::text)`, person, subjectEmail, user)

	// The ledger row a later capture would re-match the subject on. It carries
	// the address AND the display name the message arrived with.
	activity := ids.NewV7()
	mustExec(ctx, t, tx,
		`INSERT INTO activity (id, kind, occurred_at, source, captured_by)
		 VALUES ($1, 'email', now(), 'manual', 'user:'||$2::text)`, activity, user)
	mustExec(ctx, t, tx,
		`INSERT INTO capture_pending_counterparty (email, display_name, activity_id, owner_id)
		 VALUES ($1, 'Hedda Subject', $2, $3)`, subjectEmail, activity, user)

	// A custom column holding something an operator wrote about the subject —
	// declared in the CATALOG as well as added to the table, because the
	// catalog is what the scrub reads. A bare column with no catalog row is
	// invisible to it, and a fixture built that way would fail whether the code
	// was right or wrong.
	mustExec(ctx, t, tx, `ALTER TABLE person ADD COLUMN IF NOT EXISTS cf_private_note text`)
	mustExec(ctx, t, tx,
		`INSERT INTO custom_field (object, slug, label, type, column_name, created_by)
		 VALUES ('person', 'private_note', 'Private note', 'text', 'cf_private_note', $1)`, user)
	mustExec(ctx, t, tx, `UPDATE person SET cf_private_note = 'lives on Hauptstrasse' WHERE id = $1`, person)

	if err := anonymizePersonRecord(ctx, tx, person.UUID); err != nil {
		t.Fatalf("anonymizing: %v", err)
	}

	var pending int
	if err := tx.QueryRow(ctx,
		`SELECT count(*) FROM capture_pending_counterparty WHERE email = $1`, subjectEmail).Scan(&pending); err != nil {
		t.Fatal(err)
	}
	if pending != 0 {
		t.Errorf("%d pending-counterparty row(s) still name %s — the key a later capture would "+
			"re-match the subject on survived the action that stopped naming them", pending, subjectEmail)
	}

	var note *string
	if err := tx.QueryRow(ctx,
		`SELECT cf_private_note FROM person WHERE id = $1`, person).Scan(&note); err != nil {
		t.Fatal(err)
	}
	if note != nil {
		t.Errorf("a custom column still holds %q after the person was anonymized — the fixed "+
			"columns were nulled and what an operator wrote beside them was not", *note)
	}
}

// execer is what both a connection and a transaction answer, so a fixture can
// be moved inside a rollback without rewriting every call.
type execer interface {
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
}

func mustExec(ctx context.Context, t *testing.T, conn execer, sql string, args ...any) {
	t.Helper()
	if _, err := conn.Exec(ctx, sql, args...); err != nil {
		t.Fatalf("seeding (%s): %v", sql, err)
	}
}

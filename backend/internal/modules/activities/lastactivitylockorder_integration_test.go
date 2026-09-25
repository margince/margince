// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package activities

// One lock order, asked of both halves that decide it.
//
// The last-activity refresh takes FOR UPDATE on every record a link reaches,
// and two writers that take the same records in different sequences deadlock.
// Which sequence a writer takes is decided in two places — the order this
// module inserts an activity's links, and the order the trigger walks the
// records one link reaches — so both are asked here.
//
// NOT by provoking a deadlock. That would need two transactions interleaved
// INSIDE one statement, and there is no barrier there to hold one at: the
// inserts happen in a single call, and the trigger's walk is a single
// statement. What can be measured is the ORDER, which is the property that
// makes the cycle impossible, and the order is observable from outside.
//
// The fix reaches the writers that go through this module. A caller inserting
// activity_link rows by hand still chooses its own sequence, which is why the
// table's writes belong here.

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// The writer's half: links reach the database in (type, id) order whatever
// order the caller named them in, so two callers naming the same records
// cannot take them in opposite sequences.
//
// ctid is the only trace insert order leaves, and it is exactly the right one:
// the rows are written to one page in the order the statements ran, so reading
// them back by ctid reads them back in the order the locks were taken.
func TestLinksAreInsertedInOneOrderWhateverTheCallerNamed(t *testing.T) {
	e := setupSend(t)
	ctx := context.Background()

	contact := seedLinkContact(t, e, "Ada Lindqvist")
	deal := seedLinkDeal(t, e, "Fleet retrofit")
	// Named deal-first, which is the sequence that has to be reordered: a
	// caller naming contact-first would pass whether or not anything sorted.
	activity := logLinkedActivity(t, e, []ActivityLinkInput{
		{EntityType: "deal", EntityID: deal},
		{EntityType: "contact", EntityID: contact},
	})

	var order []string
	rows, err := e.owner.Query(ctx,
		`SELECT entity_type FROM activity_link WHERE activity_id = $1 ORDER BY ctid`, activity)
	if err != nil {
		t.Fatalf("reading the links back: %v", err)
	}
	defer rows.Close()
	for rows.Next() {
		var kind string
		if err := rows.Scan(&kind); err != nil {
			t.Fatalf("scanning a link: %v", err)
		}
		order = append(order, kind)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("draining the links: %v", err)
	}
	if len(order) != 2 || order[0] != "contact" || order[1] != "deal" {
		t.Fatalf("links landed as %v — a caller's order is the lock order, so two callers naming "+
			"the same records the other way round take them in opposite sequences and deadlock", order)
	}
}

// The trigger's half: the records ONE link reaches are locked in (type, id)
// order, so the company loop's claim — "two writers reaching the same accounts
// lock them in the same order" — is true of the whole reached set and not only
// of the accounts.
//
// Measured by replacing move_last_activity with a recorder INSIDE the
// transaction and rolling it back. That is the only honest way to see the
// sequence: the locks themselves leave no ordered trace, and reading the
// function's source would assert the shape of the code rather than what it
// does.
func TestTheRefreshLocksItsWholeReachedSetInOneOrder(t *testing.T) {
	e := setupSend(t)
	ctx := context.Background()

	contact := seedLinkContact(t, e, "Ida Keller")
	deal := seedLinkDeal(t, e, "Depot fit-out")
	company := seedLinkCompany(t, e, "Baqend GmbH")

	tx, err := e.owner.Begin(ctx)
	if err != nil {
		t.Fatalf("opening the recording transaction: %v", err)
	}
	defer func() {
		if err := tx.Rollback(context.Background()); err != nil && !isTxClosed(err) {
			t.Errorf("rolling the recorder back: %v", err)
		}
	}()

	if _, err := tx.Exec(ctx, `
		CREATE TEMP TABLE reached_in_order (seq serial, kind text, id uuid) ON COMMIT DROP;
		CREATE OR REPLACE FUNCTION move_last_activity(tbl regclass, rid uuid) RETURNS void
		    LANGUAGE plpgsql AS $$
		BEGIN
		  IF rid IS NULL THEN RETURN; END IF;
		  INSERT INTO reached_in_order (kind, id) VALUES (tbl::text, rid);
		END;
		$$;`); err != nil {
		t.Fatalf("installing the recorder: %v", err)
	}
	if _, err := tx.Exec(ctx,
		`SELECT refresh_last_activity_for_link($1, $2, $3)`, contact, deal, company); err != nil {
		t.Fatalf("running the refresh: %v", err)
	}

	var order []string
	rows, err := tx.Query(ctx, `SELECT kind FROM reached_in_order ORDER BY seq`)
	if err != nil {
		t.Fatalf("reading what it reached: %v", err)
	}
	defer rows.Close()
	for rows.Next() {
		var kind string
		if err := rows.Scan(&kind); err != nil {
			t.Fatalf("scanning a reached record: %v", err)
		}
		order = append(order, kind)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("draining what it reached: %v", err)
	}
	// Sorted by type then id: company, contact, deal. The positional spelling
	// gave contact, deal, company — a sequence no other writer has a reason to
	// agree with.
	if len(order) != 3 || order[0] != "company" || order[1] != "contact" || order[2] != "deal" {
		t.Fatalf("the refresh reached %v — the order is positional, so two firings that share "+
			"records take them in whichever sequence their arms happen to sit in", order)
	}
}

func isTxClosed(err error) bool { return err != nil && err.Error() == pgx.ErrTxClosed.Error() }

func seedLinkRow(t *testing.T, e *sendEnv, sql string, args ...any) {
	t.Helper()
	if _, err := e.owner.Exec(context.Background(), sql, args...); err != nil {
		t.Fatalf("seeding: %v", err)
	}
}

func seedLinkContact(t *testing.T, e *sendEnv, name string) ids.UUID {
	t.Helper()
	id := ids.NewV7()
	seedLinkRow(t, e, `INSERT INTO contact (id, full_name, owner_id, source, captured_by)
		VALUES ($1, $2, $3, 'seed', 'system')`, id, name, e.rep)
	return id
}

func seedLinkCompany(t *testing.T, e *sendEnv, name string) ids.UUID {
	t.Helper()
	id := ids.NewV7()
	seedLinkRow(t, e, `INSERT INTO company (id, display_name, owner_id, source, captured_by)
		VALUES ($1, $2, $3, 'seed', 'system')`, id, name, e.rep)
	return id
}

func seedLinkDeal(t *testing.T, e *sendEnv, name string) ids.UUID {
	t.Helper()
	id, pipeline, stage := ids.NewV7(), ids.NewV7(), ids.NewV7()
	seedLinkRow(t, e, `INSERT INTO pipeline (id, name) VALUES ($1, $2)`, pipeline, "Pipeline "+pipeline.String())
	seedLinkRow(t, e, `INSERT INTO stage (id, pipeline_id, name, "position") VALUES ($1, $2, 'Qualified', 1)`,
		stage, pipeline)
	seedLinkRow(t, e, `INSERT INTO deal (id, name, status, owner_id, pipeline_id, stage_id, source, captured_by)
		VALUES ($1, $2, 'open', $3, $4, $5, 'seed', 'system')`, id, name, e.rep, pipeline, stage)
	return id
}

// logLinkedActivity files one note through the real writer with the links
// named in the caller's order. Through LogActivity rather than a hand INSERT:
// the sort being asked about lives in the writer, and a test that inserted the
// rows itself would be asking about its own loop.
func logLinkedActivity(t *testing.T, e *sendEnv, links []ActivityLinkInput) ids.UUID {
	t.Helper()
	subject := "Depot slot"
	activity, _, err := e.store(nil).LogActivity(linkWriterCtx(e), LogActivityInput{
		Kind: "note", Subject: &subject, Source: "manual", Links: links,
	})
	if err != nil {
		t.Fatalf("LogActivity: %v", err)
	}
	return ids.UUID(activity.Id)
}

// linkWriterCtx carries the grants an activity with links needs: creating the
// row, and attaching it to a contact, a deal and a company.
func linkWriterCtx(e *sendEnv) context.Context {
	ctx := principal.WithWorkspaceID(context.Background(), e.ws)
	ctx = principal.WithCorrelationID(ctx, ids.NewV7())
	return principal.WithActor(ctx, principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:" + e.rep.String(), UserID: e.rep,
		Permissions: principal.Permissions{
			RoleKeys: []string{"rep"},
			Objects: map[string]principal.ObjectGrant{
				"activity": {Create: true, Read: true, Update: true},
				"contact":  {Read: true, Update: true},
				"deal":     {Read: true, Update: true},
				"company":  {Read: true, Update: true},
			},
			RowScope: principal.RowScopeAll,
		},
	})
}

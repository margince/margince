// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package migrations_test

// The repair for a shipped migration that was edited after it had been applied.
//
// 1788407500 first shipped `communication_decision_one_per_attempt`, then was
// edited in place to ship `communication_decision_one_per_decision`. An applied
// version never re-runs, so a database that took the first reading kept it: the
// ledger says the version is done and the runner never revisits it.
//
// Every gate in this package builds a FRESH database, where the edited file and
// the schema always agree, so all of them are green and blind to this by
// construction. This suite is the one that is not: it puts a database into the
// drifted state on purpose and replays the repair over it.
//
// It replays the shipped migration TEXT rather than a copy of its statements —
// a test written against a copy keeps passing against SQL that no longer ships.

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// undefinedObject is what Postgres answers when ON CONFLICT names a
// specification no unique index matches (42P10).
const undefinedObject = "42P10"

// driftDatabase puts the schema back the way a database that applied
// 1788407500 before it was edited actually stands: the attempt index present,
// the decision index absent.
func driftDatabase(t *testing.T, conn *pgx.Conn) {
	t.Helper()
	if _, err := conn.Exec(context.Background(), `
		DROP INDEX communication_decision_one_per_decision;
		CREATE UNIQUE INDEX communication_decision_one_per_attempt
		    ON communication_decision (delivery_id, recipient_address, phase, attempt)`); err != nil {
		t.Fatalf("putting the database into the drifted state: %v", err)
	}
}

// replayDecisionIndexRepair runs the shipped repair over whatever state the
// test left.
func replayDecisionIndexRepair(t *testing.T, conn *pgx.Conn) {
	t.Helper()
	up, err := os.ReadFile(filepath.Join("core",
		"1788497182_the_decision_index_matches_what_the_send_infers_on.up.sql"))
	if err != nil {
		t.Fatalf("reading the repair: %v", err)
	}
	if _, err := conn.Exec(context.Background(), string(up)); err != nil {
		t.Fatalf("replaying the repair: %v", err)
	}
}

// inferOnDecisionSet asks the schema the question Gate.AuthorizeTransmit's
// write asks it: does a unique index match this inference specification.
//
// Through EXPLAIN rather than a real insert. Resolving the specification is
// PLANNING work, so EXPLAIN raises the same 42P10 the send would — while
// needing no comms_outbound row for the foreign key and writing nothing. A
// prepare is not enough: it does not resolve the arbiter, and the drifted
// database answers it happily.
func inferOnDecisionSet(ctx context.Context, conn *pgx.Conn) error {
	_, err := conn.Exec(ctx, `
		EXPLAIN INSERT INTO communication_decision
			(delivery_id, decision_set_id, recipient_address, phase,
			 resolved_category, verdict, reason_code, mode, actor)
		VALUES (gen_random_uuid(), gen_random_uuid(), 'buyer@example.test', 'transmit',
		        'reply_to_inbound', 'allow', 'ok', 'enforce', 'system')
		ON CONFLICT (decision_set_id, recipient_address, phase) DO NOTHING`)
	return err
}

// The symptom, and then the repair — in one test, because the second proves
// nothing without the first. A repair asserted only against a database that was
// never broken is asserting that a no-op is a no-op.
func TestTheRepairRestoresTheInferenceASendDependsOn(t *testing.T) {
	ownerDSN, _ := dsns(t)
	conn := connect(t, ownerDSN)
	ctx := context.Background()

	driftDatabase(t, conn)

	err := inferOnDecisionSet(ctx, conn)
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.Code != undefinedObject {
		t.Fatalf("the drifted database answered %v, wanted %s — if this does not fail, "+
			"the state below is not the one the repair exists for", err, undefinedObject)
	}

	replayDecisionIndexRepair(t, conn)

	if err := inferOnDecisionSet(ctx, conn); err != nil {
		t.Fatalf("the send's inference still fails after the repair: %v", err)
	}
}

// Fresh installations run this too, and it must change nothing there: the
// decision index is already the one 1788407500 ships today, and the attempt
// index has never existed.
func TestTheRepairIsANoOpOnADatabaseThatWasNeverDrifted(t *testing.T) {
	ownerDSN, _ := dsns(t)
	conn := connect(t, ownerDSN)
	ctx := context.Background()

	// The index's identity, not merely its presence. A repair that DROPped the
	// correct index and CREATEd it again would leave a database that infers
	// perfectly and still be the wrong thing to ship: on a populated table that
	// is an exclusive lock and a full rebuild, during a migration a fresh
	// installation is told costs nothing. Only the oid separates the two, since
	// the name and the definition are identical either way.
	before := indexOID(t, ctx, conn)

	replayDecisionIndexRepair(t, conn)

	if after := indexOID(t, ctx, conn); after != before {
		t.Errorf("the repair replaced the decision index (oid %d became %d) — it is "+
			"meant to leave a correct one alone, and dropping it to recreate it takes "+
			"an exclusive lock and rebuilds every row", before, after)
	}
	if err := inferOnDecisionSet(ctx, conn); err != nil {
		t.Fatalf("the send's inference broke on an undrifted database: %v", err)
	}
	var attempt int
	if err := conn.QueryRow(ctx, `
		SELECT count(*) FROM pg_class
		 WHERE relkind = 'i' AND relname = 'communication_decision_one_per_attempt'`).
		Scan(&attempt); err != nil {
		t.Fatalf("reading the indexes: %v", err)
	}
	if attempt != 0 {
		t.Error("the retired index is present on a fresh database")
	}
}

// indexOID reads the identity of the index a send's inference depends on.
//
// Fatal when it is missing rather than answering zero: a comparison of two
// absences would be equal, and this is the one reading whose whole purpose is
// to notice a difference.
func indexOID(t *testing.T, ctx context.Context, conn *pgx.Conn) uint32 {
	t.Helper()
	var oid uint32
	if err := conn.QueryRow(ctx, `
		SELECT oid FROM pg_class
		 WHERE relkind = 'i' AND relname = 'communication_decision_one_per_decision'`).
		Scan(&oid); err != nil {
		t.Fatalf("reading the decision index's identity: %v", err)
	}
	return oid
}

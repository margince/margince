// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func seedThreadInheritance(t *testing.T, e *integration.Env, key string, bases int) {
	t.Helper()
	contact := e.SeedContact(t, "Thread correspondent", &e.Rep1)
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(e.Admin(), `INSERT INTO capture_thread_verdict(thread_key,user_id,status)
 VALUES(@key,@seat,'held_by_owner') ON CONFLICT(thread_key,user_id) DO UPDATE SET status=EXCLUDED.status`, pgx.NamedArgs{"key": key, "seat": e.Rep1}); err != nil {
			return err
		}
		_, err := tx.Exec(e.Admin(), `INSERT INTO communication_basis(contact_id,kind,thread_key,valid_from,captured_by)
 SELECT @contact,'subject_initiated_correspondence',@key,'2026-01-01T00:00:00Z'::timestamptz+make_interval(secs=>n), 'human:test'
 FROM generate_series(1,@count::integer) n`, pgx.NamedArgs{"contact": contact, "key": key, "count": bases})
		return err
	}); err != nil {
		t.Fatal(err)
	}
}

func assertThreadInheritedHold(t *testing.T, e *integration.Env, key string, activity ids.ActivityID, bases int) {
	t.Helper()
	if n := e.WsCount(t, `SELECT count(*) FROM communication_basis WHERE thread_key=$1 AND revoked_at IS NULL`, key); n != bases {
		t.Fatalf("thread %s inherited %d bases, want %d", key, n, bases)
	}
	if got := e.WsScalar(t, `SELECT status FROM capture_thread_verdict WHERE thread_key=$1 AND user_id=$2`, key, e.Rep1); got != capture.VerdictHeldByOwner {
		t.Fatalf("inherited ledger verdict=%q", got)
	}
	if got := e.WsScalar(t, `SELECT verdict_status FROM capture_import WHERE activity_id=$1 AND user_id=$2`, activity, e.Rep1); got != capture.VerdictHeldByOwner {
		t.Fatalf("inherited current-message verdict=%q", got)
	}
	if got := e.WsScalar(t, `SELECT audience FROM activity WHERE id=$1`, activity); got != "participants" {
		t.Fatalf("closed thread audience=%q", got)
	}
}

func TestALateParentCarriesClosedHoldsAndActiveBasesAcrossATopicBoundary(t *testing.T) {
	e := integration.Setup(t)
	const parent = "late-private-parent@counterparty.example"
	const child = "early-private-child@counterparty.example"
	seedThreadInheritance(t, e, parent, 3)
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		_, err := tx.Exec(e.Admin(), `INSERT INTO communication_basis(contact_id,kind,thread_key,valid_from,valid_until,revoked_at,captured_by)
 SELECT b.contact_id,b.kind,b.thread_key,w.starts,w.ends,w.revoked,b.captured_by
 FROM (SELECT * FROM communication_basis WHERE thread_key=@key ORDER BY id LIMIT 1) b
 CROSS JOIN (VALUES
 ('1900-01-01'::timestamptz,'1901-01-01'::timestamptz,NULL::timestamptz),
 ('2999-01-01'::timestamptz,NULL::timestamptz,NULL::timestamptz),
 ('1902-01-01'::timestamptz,NULL::timestamptz,'1903-01-01'::timestamptz)) w(starts,ends,revoked)`, pgx.NamedArgs{"key": parent})
		return err
	}); err != nil {
		t.Fatal(err)
	}
	addr := seatAddress(t, e, e.Rep1)
	captureThreadMail(t, e, e.Rep1, subjectThreadMail(addr, child, "Logo", parent))
	captureThreadMail(t, e, e.Rep1, subjectThreadMail(addr, parent, "Price", ""))
	finishThreadRepairs(t, e)
	id := capturedThreadActivity(t, e, child)
	assertThreadInheritedHold(t, e, child, id, 3)
	if got := threadKeyOfMessage(t, e, child); got != child {
		t.Fatalf("changed topic grouped as %q", got)
	}
	if n := e.WsCount(t, `SELECT count(*) FROM communication_basis WHERE thread_key=$1`, parent); n != 6 {
		t.Fatalf("source retained %d bases", n)
	}
}

func TestALargeBirthInheritanceStaysUnavailableUntilAllBasesPrecedeAttribution(t *testing.T) {
	e := integration.Setup(t)
	e.GrantRole(t, e.Rep1, "admin")
	const parent = "many-bases-parent@counterparty.example"
	const child = "many-bases-child@counterparty.example"
	const bases = 257
	addr := seatAddress(t, e, e.Rep1)
	captureThreadMail(t, e, e.Rep1, subjectThreadMail(addr, parent, "Price", ""))
	seedThreadInheritance(t, e, parent, bases)
	account := seedLadderAccount(t, e)
	project, key := account.project(t, "Inherited grounds")
	captureThreadMail(t, e, e.Rep1, subjectThreadMail(addr, child, "["+key+"] Logo", parent))
	id := capturedThreadActivity(t, e, child)
	if n := e.WsCount(t, `SELECT count(*) FROM activity WHERE id=$1 AND activity_thread_available(thread_key)`, id); n != 0 {
		t.Fatal("newborn became available before bounded inheritance finished")
	}
	if got := filedUnder(t, e, id.UUID); !got.IsZero() {
		t.Fatalf("unpublished mail already filed under %s", got)
	}
	apply := threadRepairApplier(e.Pool)
	publish := apply.PublishSeat
	observed := false
	apply.PublishSeat = func(ctx context.Context, tx pgx.Tx, activity ids.ActivityID, seat ids.UUID) error {
		if activity == id {
			var count int
			if err := tx.QueryRow(ctx, `SELECT count(*) FROM communication_basis WHERE thread_key=@key AND revoked_at IS NULL`, pgx.NamedArgs{"key": child}).Scan(&count); err != nil {
				return err
			}
			if count != bases {
				t.Fatalf("attribution began with only %d/%d bases", count, bases)
			}
			observed = true
		}
		return publish(ctx, tx, activity, seat)
	}
	finished := false
	for range 4000 {
		if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
			_, _, done, err := capture.ThreadRepairBatchTx(e.Admin(), tx, 7, ids.NewV7(), apply)
			finished = done
			return err
		}); err != nil {
			t.Fatal(err)
		}
		if finished {
			break
		}
	}
	if !finished {
		t.Fatal("large inheritance did not finish")
	}
	if !observed {
		t.Fatal("published child never reached real attribution")
	}
	assertThreadInheritedHold(t, e, child, id, bases)
	if got := filedUnder(t, e, id.UUID); got != project {
		t.Fatalf("published child filed under %s, want %s", got, project)
	}
	if n := e.WsCount(t, `SELECT count(*) FROM activity WHERE id=$1 AND activity_thread_available(thread_key)`, id); n != 1 {
		t.Fatal("completed newborn is still unavailable")
	}
}

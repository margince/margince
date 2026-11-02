// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/modules/comms"
	"github.com/margince/margince/backend/internal/modules/consent"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestEveryThreadKeyedTableHasAPartitionPolicy(t *testing.T) {
	tables := threadKeyedTables(t)
	policies := slices.Sorted(slices.Values(threadSplitTables))
	if !slices.Equal(tables, policies) {
		t.Fatalf("thread tables %v; partition policies %v", tables, policies)
	}
}

func subjectThreadMail(to, key, subject, parent string) []byte {
	return []byte(strings.Replace(string(threadMail(to, key, "Mon, 01 Jun 2026 08:00:00 +0000", parent, parent)), "Subject: Re: Angebot", "Subject: "+subject, 1))
}

func capturedThreadActivity(t *testing.T, e *integration.Env, key string) ids.ActivityID {
	t.Helper()
	var id ids.ActivityID
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(), `SELECT activity_id FROM activity_identity WHERE identity_kind='mail' AND identity_key=$1`, key).Scan(&id)
	}); err != nil {
		t.Fatal(err)
	}
	return id
}

type threadOwnerFixture struct {
	*integration.Env
	first, second       ids.ActivityID
	source, destination string
	delivery, basis     ids.UUID
}

func newThreadOwnerFixture(t *testing.T) threadOwnerFixture {
	t.Helper()
	e := integration.Setup(t)
	f := threadOwnerFixture{Env: e, source: "first@counterparty.example", destination: "second@counterparty.example"}
	addr := seatAddress(t, e, e.Rep1)
	captureThreadMail(t, e, e.Rep1, subjectThreadMail(addr, f.source, "Price", ""))
	captureThreadMail(t, e, e.Rep1, subjectThreadMail(addr, f.destination, "Logo", f.source))
	f.first = capturedThreadActivity(t, e, f.source)
	f.second = capturedThreadActivity(t, e, f.destination)
	oldKey := threadKeyOfMessage(t, e, f.destination)
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		return mergeThreadsTx(e.Admin(), tx, oldKey, f.source)
	}); err != nil {
		t.Fatal(err)
	}
	contact := e.SeedContact(t, "Correspondent", &e.Rep1)
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		ctx := e.Admin()
		var err error
		f.delivery, err = comms.NewStore(e.DB(), func() time.Time { return time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC) }, nil).StageTx(ctx, tx, comms.StageInput{
			ActivityID: f.second, Provider: "gmail", MessageID: "sent-copy@example.test", Recipients: []string{"pat@counterparty.example"}, Subject: "Logo", Body: "Text", ConsentPurpose: "transactional", ThreadKey: f.source,
		})
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO capture_thread_verdict(thread_key,user_id,status,first_activity_id)
 VALUES($1,$3,'held_by_owner',$4),($2,$3,'cleared',$4)
 ON CONFLICT(thread_key,user_id) DO UPDATE SET status=EXCLUDED.status,first_activity_id=EXCLUDED.first_activity_id`, f.source, f.destination, e.Rep1, f.second); err != nil {
			return err
		}
		if err := tx.QueryRow(ctx, `INSERT INTO communication_basis(contact_id,kind,thread_key,source_activity_id,valid_from,captured_by)
 VALUES($1,'subject_initiated_correspondence',$2,$3,'2026-01-01T00:00:00Z','human:test') RETURNING id`, contact, f.source, f.first).Scan(&f.basis); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO activity_sales_state(thread_key,kind,set_by) VALUES($1,'email','human:test') ON CONFLICT DO NOTHING`, f.source); err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `INSERT INTO signal_thread_scan(thread_key,last_activity_at) VALUES($1,'2026-06-01T00:00:00Z'),($2,'2026-06-01T00:00:00Z') ON CONFLICT DO NOTHING`, f.source, f.destination)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	return f
}

func (f threadOwnerFixture) split(ctx context.Context, tx pgx.Tx) error {
	if _, err := splitThreadActivityTx(ctx, tx, f.second, f.source, f.destination); err != nil {
		return err
	}
	if err := splitThreadDeliveryTx(ctx, tx, f.delivery, f.second, f.source, f.destination); err != nil {
		return err
	}
	if err := inheritThreadSeatTx(ctx, tx, f.source, f.destination, f.Rep1); err != nil {
		return err
	}
	if err := repairThreadSeatTx(ctx, tx, f.source, f.Rep1); err != nil {
		return err
	}
	return inheritThreadBasisTx(ctx, tx, f.basis, f.source, f.destination)
}

func TestThreadOwnerMovesAreAtomicAndIdempotent(t *testing.T) {
	f := newThreadOwnerFixture(t)
	interrupted := errors.New("interrupted repair")
	if err := database.WithWorkspaceTx(f.Admin(), f.Pool, func(tx pgx.Tx) error {
		if err := f.split(f.Admin(), tx); err != nil {
			return err
		}
		return interrupted
	}); !errors.Is(err, interrupted) {
		t.Fatalf("rollback result: %v", err)
	}
	if got := threadKeyOfMessage(t, f.Env, f.destination); got != f.source {
		t.Fatalf("rollback moved message to %q", got)
	}
	for range 2 {
		if err := database.WithWorkspaceTx(f.Admin(), f.Pool, func(tx pgx.Tx) error { return f.split(f.Admin(), tx) }); err != nil {
			t.Fatal(err)
		}
	}
	if got := threadKeyOfMessage(t, f.Env, f.destination); got != f.destination {
		t.Fatalf("message on %q", got)
	}
	if err := database.WithWorkspaceTx(f.Admin(), f.Pool, func(tx pgx.Tx) error {
		var delivery string
		var basis, seats, sales, scans, audits, events int
		if err := tx.QueryRow(f.Admin(), `SELECT thread_key FROM comms_outbound WHERE id=$1`, f.delivery).Scan(&delivery); err != nil {
			return err
		}
		if err := tx.QueryRow(f.Admin(), `SELECT
 (SELECT count(*) FROM communication_basis WHERE thread_key=ANY($1::text[])),
 (SELECT count(*) FROM capture_thread_verdict WHERE thread_key=ANY($1::text[]) AND user_id=$2 AND status='held_by_owner'),
 (SELECT count(*) FROM activity_sales_state WHERE thread_key=$3),
 (SELECT count(*) FROM signal_thread_scan WHERE thread_key=ANY($1::text[])),
 (SELECT count(*) FROM audit_log WHERE entity_id=$4 AND after->>'thread_key'=$5),
 (SELECT count(*) FROM event_outbox o JOIN audit_log a ON o.envelope->'trace'->>'audit_log_id'=a.id::text WHERE a.entity_id=$4 AND a.after->>'thread_key'=$5 AND o.envelope->>'type'='activity.updated' AND o.envelope->'payload'->'changed_fields'->>'thread_key'=$5)`, []string{f.source, f.destination}, f.Rep1, f.source, f.second, f.destination).Scan(&basis, &seats, &sales, &scans, &audits, &events); err != nil {
			return err
		}
		if delivery != f.destination || basis != 2 || seats != 2 || sales != 1 || scans != 0 || audits != 1 || events != 1 {
			t.Fatalf("delivery=%q basis=%d holds=%d sales=%d scans=%d audits=%d events=%d", delivery, basis, seats, sales, scans, audits, events)
		}
		var source, destination ids.ActivityID
		if err := tx.QueryRow(f.Admin(), `SELECT first_activity_id FROM capture_thread_verdict WHERE thread_key=$1 AND user_id=$2`, f.source, f.Rep1).Scan(&source); err != nil {
			return err
		}
		if err := tx.QueryRow(f.Admin(), `SELECT first_activity_id FROM capture_thread_verdict WHERE thread_key=$1 AND user_id=$2`, f.destination, f.Rep1).Scan(&destination); err != nil {
			return err
		}
		if source != f.first || destination != f.second {
			t.Fatalf("representatives source=%s destination=%s", source, destination)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func TestThreadEdgesRequireSharedOwnership(t *testing.T) {
	f := newThreadOwnerFixture(t)
	addr := seatAddress(t, f.Env, f.Rep2)
	other := "other@counterparty.example"
	captureThreadMail(t, f.Env, f.Rep2, subjectThreadMail(addr, other, "Logo", f.source))
	third := capturedThreadActivity(t, f.Env, other)
	if err := database.WithWorkspaceTx(f.Admin(), f.Pool, func(tx pgx.Tx) error {
		common, err := activities.ThreadEdgeAuthorizedTx(f.Admin(), tx, f.first, f.second)
		if err != nil {
			return err
		}
		if !common {
			t.Fatal("same mailbox rejected")
		}
		foreign, err := activities.ThreadEdgeAuthorizedTx(f.Admin(), tx, f.first, third)
		if err != nil {
			return err
		}
		if foreign {
			t.Fatal("different mailboxes joined")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func TestThreadRepairPublishesOnlyACompleteLimitOnePartition(t *testing.T) {
	f := newThreadOwnerFixture(t)
	run := ids.NewV7()
	if err := database.WithWorkspaceTx(f.Admin(), f.Pool, func(tx pgx.Tx) error {
		return capture.QueueThreadRepairTx(f.Admin(), tx, f.source)
	}); err != nil {
		t.Fatal(err)
	}
	moved, steps := 0, 0
	for ; steps < 1000; steps++ {
		done := false
		if err := database.WithWorkspaceTx(f.Admin(), f.Pool, func(tx pgx.Tx) error {
			scanned, changed, finished, err := capture.ThreadRepairBatchTx(f.Admin(), tx, 1, run, threadRepairApplier(f.Pool))
			if err != nil {
				return err
			}
			if scanned > 1 {
				t.Fatalf("limit-one batch scanned %d", scanned)
			}
			moved += changed
			done = finished
			var active, sourceAvailable, targetAvailable bool
			if err := tx.QueryRow(f.Admin(), `SELECT
 EXISTS(SELECT 1 FROM capture_thread_repair WHERE phase<>'published'),
 activity_thread_available($1),activity_thread_available($2)`, f.source, f.destination).Scan(&active, &sourceAvailable, &targetAvailable); err != nil {
				return err
			}
			if active && (sourceAvailable || targetAvailable) {
				// The target becomes blocked when its partition is first derived, before any move.
				var applied bool
				if err := tx.QueryRow(f.Admin(), `SELECT EXISTS(SELECT 1 FROM activity WHERE id=$1 AND thread_key=$2)`, f.second, f.destination).Scan(&applied); err != nil {
					return err
				}
				if sourceAvailable || applied {
					t.Fatalf("partially published: source=%v target=%v applied=%v", sourceAvailable, targetAvailable, applied)
				}
			}
			return nil
		}); err != nil {
			t.Fatal(err)
		}
		if done {
			break
		}
	}
	if steps == 1000 {
		t.Fatal("bounded repair did not finish")
	}
	if moved != 1 {
		t.Fatalf("moved %d messages, want one", moved)
	}
	if got := threadKeyOfMessage(t, f.Env, f.destination); got != f.destination {
		t.Fatalf("repaired key=%q", got)
	}
	if err := database.WithWorkspaceTx(f.Admin(), f.Pool, func(tx pgx.Tx) error {
		return capture.QueueThreadRepairTx(f.Admin(), tx, f.source)
	}); err != nil {
		t.Fatal(err)
	}
	replayMoved := 0
	for range 1000 {
		done := false
		if err := database.WithWorkspaceTx(f.Admin(), f.Pool, func(tx pgx.Tx) error {
			_, changed, finished, err := capture.ThreadRepairBatchTx(f.Admin(), tx, 1, run, threadRepairApplier(f.Pool))
			replayMoved += changed
			done = finished
			return err
		}); err != nil {
			t.Fatal(err)
		}
		if done {
			if replayMoved != 0 {
				t.Fatalf("replay moved %d messages", replayMoved)
			}
			return
		}
	}
	t.Fatal("replay did not finish")
}

func TestThreadEdgeProvesAHumansSentMessageBeforeItsMailboxEcho(t *testing.T) {
	e := setupVoiceSend(t)
	addr := seatAddress(t, e.Env, e.Rep1)
	parent := "sent-parent@counterparty.example"
	captureThreadMail(t, e.Env, e.Rep1, subjectThreadMail(addr, parent, "Pricing", ""))
	parentID := capturedThreadActivity(t, e.Env, parent)
	stager := &recordingStager{}
	store := sendStore(e.Pool, SendPath{PublicBaseURL: voiceSendBaseURL, Delivery: stager})
	sent, err := store.SendEmail(e.ctx, activities.FromActivity(parentID), activities.SendEmailInput{
		Recipients: []string{e.recipient}, Subject: "Pricing", Body: "Here is the requested answer.", ConsentPurpose: "transactional",
	}, consent.NewGate(consent.NewStore(e.DB())), stager)
	if err != nil {
		t.Fatal(err)
	}
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		proved, err := activities.ThreadEdgeAuthorizedTx(e.Admin(), tx, ids.From[ids.ActivityKind](ids.UUID(sent.Id)), parentID)
		if err != nil {
			return err
		}
		if !proved {
			t.Fatal("genuine sent activity did not prove common ownership before sync")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

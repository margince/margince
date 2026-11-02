// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"fmt"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func finishThreadRepairs(t *testing.T, e *integration.Env) {
	t.Helper()
	for range 4000 {
		done := false
		err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
			_, _, complete, err := capture.ThreadRepairBatchTx(e.Admin(), tx, 64, ids.NewV7(), threadRepairApplier(e.Pool))
			done = complete
			return err
		})
		if err != nil {
			t.Fatal(err)
		}
		if done {
			if failures := e.WsCount(t, `SELECT (SELECT count(*) FROM capture_thread_repair WHERE state<>'ready')+(SELECT count(*) FROM capture_thread_invalidation)`); failures != 0 {
				t.Fatalf("repair drain left %d failed components", failures)
			}
			return
		}
	}
	t.Fatal("thread repair did not converge")
}

func TestSubjectPartitionsDoNotDependOnArrivalOrderOrSenderClocks(t *testing.T) {
	cases := []struct {
		name                   string
		topics, parents, roots [3]string
	}{
		{"returning topic", [3]string{"Price", "Logo", "Price"}, [3]string{"", "a", "b"}, [3]string{"a", "b", "c"}},
		{"same topic", [3]string{"Price", "Re: Price", "WG: Re: Price"}, [3]string{"", "a", "b"}, [3]string{"a", "a", "a"}},
		{"blank intermediary", [3]string{"Price", "", "Logo"}, [3]string{"", "a", "b"}, [3]string{"a", "a", "c"}},
		{"blank branch root", [3]string{"", "Price", "Logo"}, [3]string{"", "a", "a"}, [3]string{"a", "a", "c"}},
		{"cycle", [3]string{"Price", "Price", "Price"}, [3]string{"b", "c", "a"}, [3]string{"a", "a", "a"}},
		{"topic boundary in cycle", [3]string{"Price", "Logo", "Price"}, [3]string{"b", "c", "a"}, [3]string{"a", "b", "a"}},
	}
	for _, tc := range cases {
		for _, order := range [][3]int{{0, 1, 2}, {0, 2, 1}, {1, 0, 2}, {1, 2, 0}, {2, 0, 1}, {2, 1, 0}} {
			t.Run(fmt.Sprintf("%s/%v", tc.name, order), func(t *testing.T) {
				e := integration.Setup(t)
				addr := seatAddress(t, e, e.Rep1)
				for _, i := range order {
					key := string(rune('a'+i)) + "@counterparty.example"
					parent := tc.parents[i]
					if parent != "" {
						parent += "@counterparty.example"
					}
					raw := subjectThreadMail(addr, key, tc.topics[i], parent)
					if i == 0 {
						raw = []byte(strings.Replace(string(raw), "Mon, 01 Jun 2026", "Wed, 03 Jun 2026", 1))
					}
					captureThreadMail(t, e, e.Rep1, raw)
				}
				finishThreadRepairs(t, e)
				for i, want := range tc.roots {
					key := string(rune('a'+i)) + "@counterparty.example"
					if got := threadKeyOfMessage(t, e, key); got != want+"@counterparty.example" {
						t.Fatalf("%s root=%s want=%s", key, got, want)
					}
				}
			})
		}
	}
}

func TestAMissingDirectParentDoesNotJumpAcrossAnEarlierReference(t *testing.T) {
	e := integration.Setup(t)
	addr := seatAddress(t, e, e.Rep1)
	const a = "a@counterparty.example"
	const b = "b@counterparty.example"
	const c = "c@counterparty.example"
	captureThreadMail(t, e, e.Rep1, subjectThreadMail(addr, a, "Price", ""))
	raw := threadMail(addr, c, "Mon, 01 Jun 2026 08:00:00 +0000", b, a, b)
	raw = []byte(strings.Replace(string(raw), "Subject: Re: Angebot", "Subject: Price", 1))
	captureThreadMail(t, e, e.Rep1, raw)
	finishThreadRepairs(t, e)
	if got := threadKeyOfMessage(t, e, c); got != c {
		t.Fatalf("missing direct parent skipped to %s", got)
	}
	captureThreadMail(t, e, e.Rep1, subjectThreadMail(addr, b, "Logo", a))
	finishThreadRepairs(t, e)
	for _, key := range []string{a, b, c} {
		if got := threadKeyOfMessage(t, e, key); got != key {
			t.Fatalf("%s joined %s across a subject boundary", key, got)
		}
	}
}

func TestAnUnprovenBracketTagDoesNotFailCaptureOrJoinAnotherTopic(t *testing.T) {
	e := integration.Setup(t)
	addr := seatAddress(t, e, e.Rep1)
	const a = "tag-a@counterparty.example"
	const b = "tag-b@counterparty.example"
	captureThreadMail(t, e, e.Rep1, subjectThreadMail(addr, a, "Price", ""))
	captureThreadMail(t, e, e.Rep1, subjectThreadMail(addr, b, "[FYI] Price", a))
	finishThreadRepairs(t, e)
	if got := threadKeyOfMessage(t, e, b); got != b {
		t.Fatalf("unproven tag was stripped: root=%s", got)
	}
}

func TestAChangedVersionRestartsUnpublishedThreadPlanning(t *testing.T) {
	e := integration.Setup(t)
	addr := seatAddress(t, e, e.Rep1)
	const a = "late-a@counterparty.example"
	const b = "late-b@counterparty.example"
	captureThreadMail(t, e, e.Rep1, subjectThreadMail(addr, a, "Price", ""))
	captureThreadMail(t, e, e.Rep1, subjectThreadMail(addr, b, "Price", a))
	finishThreadRepairs(t, e)
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error { return capture.QueueThreadRepairTx(e.Admin(), tx, a) }); err != nil {
		t.Fatal(err)
	}
	reachedApply := false
	for range 1000 {
		phase := ""
		err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
			if _, _, _, err := capture.ThreadRepairBatchTx(e.Admin(), tx, 1, ids.NewV7(), threadRepairApplier(e.Pool)); err != nil {
				return err
			}
			return tx.QueryRow(e.Admin(), `SELECT phase FROM capture_thread_repair WHERE phase<>'published' LIMIT 1`).Scan(&phase)
		})
		if err != nil {
			t.Fatal(err)
		}
		if phase == "apply" {
			reachedApply = true
			break
		}
	}
	if !reachedApply {
		t.Fatal("repair never reached unpublished application")
	}
	id := capturedThreadActivity(t, e, b)
	// A writer admitted before the barrier may commit after planning read its version.
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		_, err := tx.Exec(e.Admin(), `UPDATE activity SET subject='Logo' WHERE id=@id`, pgx.NamedArgs{"id": id})
		return err
	}); err != nil {
		t.Fatal(err)
	}
	finishThreadRepairs(t, e)
	if got := threadKeyOfMessage(t, e, a); got != a {
		t.Fatalf("source root=%s", got)
	}
	if got := threadKeyOfMessage(t, e, b); got != b {
		t.Fatalf("stale plan survived changed version: root=%s", got)
	}
}

func TestAnUnchangedCaptureReplayDoesNotRepublishItsConversation(t *testing.T) {
	e := integration.Setup(t)
	addr := seatAddress(t, e, e.Rep1)
	const key = "replayed-root@counterparty.example"
	raw := subjectThreadMail(addr, key, "Price", "")
	captureThreadMail(t, e, e.Rep1, raw)
	id := capturedThreadActivity(t, e, key)
	auditBefore := e.WsCount(t, `SELECT count(*) FROM audit_log WHERE entity_type='activity' AND entity_id=$1 AND after ? 'version' AND after-'version'='{}'::jsonb`, id)
	eventsBefore := e.WsCount(t, `SELECT count(*) FROM event_outbox WHERE envelope->'entity'->>'id'=$1 AND envelope->'payload'->'changed_fields' ? 'thread_key'`, id.String())
	captureThreadMail(t, e, e.Rep1, raw)
	if after := e.WsCount(t, `SELECT count(*) FROM audit_log WHERE entity_type='activity' AND entity_id=$1 AND after ? 'version' AND after-'version'='{}'::jsonb`, id); after != auditBefore {
		t.Fatalf("replay publication audits=%d, before=%d", after, auditBefore)
	}
	if after := e.WsCount(t, `SELECT count(*) FROM event_outbox WHERE envelope->'entity'->>'id'=$1 AND envelope->'payload'->'changed_fields' ? 'thread_key'`, id.String()); after != eventsBefore {
		t.Fatalf("replay publication events=%d, before=%d", after, eventsBefore)
	}
}

func TestAResumedThreadRepairKeepsItsDurableAuditCorrelation(t *testing.T) {
	f := newThreadOwnerFixture(t)
	ctx := principal.SystemActing(f.Admin(), "system:thread-trace-test")
	idsOfMessages := []ids.UUID{f.first.UUID, f.second.UUID}
	const threadEvents = `SELECT count(*) FROM event_outbox WHERE envelope->'entity'->>'id'=ANY(SELECT unnest($1::uuid[])::text) AND envelope->'payload'->'changed_fields' ? 'thread_key'`
	before := f.WsCount(t, threadEvents, idsOfMessages)
	if err := database.WithWorkspaceTx(ctx, f.Pool, func(tx pgx.Tx) error { return capture.QueueThreadRepairTx(ctx, tx, f.source) }); err != nil {
		t.Fatal(err)
	}
	run := ids.NewV7()
	complete := false
	for range 2000 {
		err := database.WithWorkspaceTx(ctx, f.Pool, func(tx pgx.Tx) error {
			_, _, done, err := capture.ThreadRepairBatchTx(principal.WithCorrelationID(ctx, ids.NewV7()), tx, 1, run, threadRepairApplier())
			complete = done
			return err
		})
		if err != nil {
			t.Fatal(err)
		}
		if complete {
			break
		}
	}
	if !complete {
		t.Fatal("repair did not finish")
	}
	added := f.WsCount(t, threadEvents, idsOfMessages) - before
	correlated := f.WsCount(t, threadEvents+` AND envelope->'trace'->>'correlation_id'=$2`, idsOfMessages, run.String())
	if added == 0 || correlated != added {
		t.Fatalf("repair events=%d; durable correlation events=%d", added, correlated)
	}
}

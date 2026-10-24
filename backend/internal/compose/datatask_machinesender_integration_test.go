// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/search"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/datatask"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

type senderTaskFixture struct {
	e                 *integration.Env
	seat, contact     ids.UUID
	notice, human     ids.UUID
	seatAddr, contAdr string
}

func senderMessage(key, from, to, cc string, at time.Time) []byte {
	return []byte(strings.Join([]string{
		"From: " + from, "To: " + to, "Cc: " + cc, "Subject: " + key,
		"Date: " + at.Format(time.RFC1123Z), "Message-ID: <" + key + "@sender-task.test>",
		"Content-Type: text/plain; charset=utf-8", "", "About " + key + ".", "",
	}, "\r\n"))
}

func activityBySubject(t *testing.T, e *integration.Env, subject string) ids.UUID {
	t.Helper()
	var id ids.UUID
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(), `SELECT id FROM activity WHERE subject = $1`, subject).Scan(&id)
	}); err != nil {
		t.Fatalf("reading %q: %v", subject, err)
	}
	return id
}

// newSenderTaskFixture captures a file-share notice and a human's mail, then
// clears the stamp, as mail captured before capture stamped it.
func newSenderTaskFixture(t *testing.T) senderTaskFixture {
	t.Helper()
	e := integration.Setup(t)
	f := senderTaskFixture{e: e, seat: e.Rep1, contAdr: "dana@client.test"}
	f.seatAddr = sharedMailbox(t, e, f.seat)
	made, err := e.Contacts.CreateContact(e.Admin(), contacts.CreateContactInput{
		FullName: "Dana Client", Source: "manual",
		Emails: []contacts.ContactEmailInput{{Email: f.contAdr, EmailType: "work", IsPrimary: true}},
	})
	if err != nil {
		t.Fatal(err)
	}
	f.contact = ids.UUID(made.Id)
	at := time.Now().UTC().AddDate(0, 0, -3).Truncate(time.Second)
	captureMailVia(t, e, f.seat, "gmail", senderMessage("deck-shared", "drive-shares-dm-noreply@google.com", f.contAdr, f.seatAddr, at))
	captureMailVia(t, e, f.seat, "gmail", senderMessage("dana-writes", f.contAdr, f.seatAddr, "", at))
	f.notice, f.human = activityBySubject(t, e, "deck-shared"), activityBySubject(t, e, "dana-writes")
	if !f.stamped(t, f.notice) {
		t.Fatal("capture did not stamp the notice; the fixture would test nothing")
	}
	if _, err := integration.OwnerConn(t).Exec(context.Background(), `UPDATE activity SET sender_is_machine = false WHERE id = $1`, f.notice); err != nil {
		t.Fatal(err)
	}
	f.rebuildEdges(t)
	return f
}

func (f senderTaskFixture) stamped(t *testing.T, activity ids.UUID) bool {
	t.Helper()
	var stamped bool
	if err := database.WithWorkspaceTx(f.e.Admin(), f.e.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(), `SELECT sender_is_machine FROM activity WHERE id = $1`, activity).Scan(&stamped)
	}); err != nil {
		t.Fatal(err)
	}
	return stamped
}

func (f senderTaskFixture) rebuildEdges(t *testing.T) {
	t.Helper()
	if err := database.WithWorkspaceTx(f.e.Admin(), f.e.Pool, func(tx pgx.Tx) error {
		if _, err := f.e.Contacts.PromoteContactCohortTx(f.e.Admin(), tx, ids.ContactID{UUID: f.contact}); err != nil {
			return err
		}
		return search.RebuildEdges(f.e.Admin(), tx)
	}); err != nil {
		t.Fatal(err)
	}
}

func (f senderTaskFixture) edgeCount(t *testing.T) int {
	t.Helper()
	var count int
	if err := database.WithWorkspaceTx(f.e.Admin(), f.e.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(), `SELECT coalesce(max(count_90d), 0) FROM graph_interaction_edge
			 WHERE user_id = $1 AND contact_id = $2`, f.seat, f.contact).Scan(&count)
	}); err != nil {
		t.Fatal(err)
	}
	return count
}

func TestTheSenderTaskMarksOnlyASendingSystemsMailAndReplaysSafely(t *testing.T) {
	f := newSenderTaskFixture(t)
	if got := f.edgeCount(t); got != 2 {
		t.Fatalf("before the task the edge counts %d exchanges, want the stored 2 the old rule counted", got)
	}
	ctx := principal.SystemActing(f.e.Admin(), "system:datatask-test")
	runMachineAddressTask(ctx, t, f.e, machineSenderDataTask())

	if !f.stamped(t, f.notice) {
		t.Error("the file-share notice is still read as a human's mail")
	}
	if f.stamped(t, f.human) {
		t.Error("the contact's own mail was marked as a sending system's")
	}
	if got := f.edgeCount(t); got != 1 {
		t.Errorf("after the task the edge counts %d exchanges, want 1: the stored edge was not folded again", got)
	}

	step := machineSenderDataTask().Steps[0]
	if err := database.WithWorkspaceTx(ctx, f.e.Pool, func(tx pgx.Tx) error {
		out, err := step.Batch(ctx, tx, datatask.BatchIn{Limit: 10, RunID: ids.NewV7()})
		if err == nil && out.Changed != 0 {
			t.Errorf("a second run changed %d row(s)", out.Changed)
		}
		return err
	}); err != nil {
		t.Fatal(err)
	}
}

func TestTheSenderTaskRollsBackWithItsBatch(t *testing.T) {
	f := newSenderTaskFixture(t)
	ctx := principal.SystemActing(f.e.Admin(), "system:datatask-test")
	step := machineSenderDataTask().Steps[0]
	rollback := errors.New("roll back after the write")
	err := database.WithWorkspaceTx(ctx, f.e.Pool, func(tx pgx.Tx) error {
		out, err := step.Batch(ctx, tx, datatask.BatchIn{Limit: 10, RunID: ids.NewV7()})
		if err != nil {
			return err
		}
		if out.Changed != 1 {
			t.Errorf("the batch changed %d row(s), want the one notice", out.Changed)
		}
		return rollback
	})
	if !errors.Is(err, rollback) {
		t.Fatal(err)
	}
	if f.stamped(t, f.notice) {
		t.Error("a rolled-back batch left its mark")
	}
	count, more, err := step.Estimate(principal.SystemActing(f.e.Admin(), "system:datatask-test"), f.e.Pool, 1)
	if err != nil {
		t.Fatal(err)
	}
	if count != 1 || !more {
		t.Errorf("the estimate capped at 1 reads (%d, more=%v), want (1, true) over two received mails", count, more)
	}
}

// An installation that finished version 1 of the task keeps its ledger row, so
// only a higher version makes the cursor start over and stamp the notices the
// first run did not recognise.
func TestTheSenderTaskRunsAgainOverAFinishedFirstVersion(t *testing.T) {
	f := newSenderTaskFixture(t)
	ctx := principal.SystemActing(f.e.Admin(), "system:datatask-test")

	finished := machineSenderDataTask()
	finished.Version = 1
	runMachineAddressTask(ctx, t, f.e, finished)
	if _, err := integration.OwnerConn(t).Exec(ctx, `UPDATE activity SET sender_is_machine = false WHERE id = $1`, f.notice); err != nil {
		t.Fatal(err)
	}

	runMachineAddressTask(ctx, t, f.e, machineSenderDataTask())
	if !f.stamped(t, f.notice) {
		t.Error("the notice stayed unmarked after the version bump")
	}
}

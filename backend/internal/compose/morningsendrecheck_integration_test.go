// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/testdb"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// The morning mail is read twice: once to decide it is worth sending, and
// again after its one send is reserved. A task handed to a colleague in
// between is not mailed to its former holder; work still theirs is mailed
// once.
func TestAMorningMailRereadsTheWorkAfterItsSendIsReserved(t *testing.T) {
	relay := &countingMailer{}
	b := setupBriefJob(t).withMailer(relay)
	morning := time.Date(2026, 6, 4, 9, 0, 0, 0, time.UTC)
	b.now = morning
	assignee := ids.From[ids.UserKind](b.Rep1)
	kept, moved := "Send the revised Globex quote", "Call Initech about the renewal"
	seedWorkFor(t, b.Env, morning, b.Rep1)
	task, _, err := b.Activities.LogActivity(b.Admin(), activities.LogActivityInput{Kind: "task", Subject: &moved, AssigneeID: &assignee, DueAt: &morning, Source: "manual"})
	if err != nil {
		t.Fatal(err)
	}
	ctx := b.As(b.Rep1, []ids.UUID{b.Team1}, integration.AdminPerms)
	candidate, ready, err := b.worker.prepareMorningMail(ctx, morning)
	if err != nil || !ready {
		t.Fatalf("the morning was not ready to send: %v", err)
	}
	if !strings.Contains(candidate.body.Text, moved) {
		t.Fatal("the first read did not include the task, so the test proves nothing")
	}
	colleague := ids.From[ids.UserKind](b.Rep2)
	if _, err := b.Activities.UpdateActivity(b.Admin(), ids.From[ids.ActivityKind](ids.UUID(task.Id)), activities.UpdateActivityInput{AssigneeID: &colleague}); err != nil {
		t.Fatal(err)
	}
	b.worker.sendMorning(ctx, candidate, morning)
	b.worker.sendMorning(ctx, candidate, morning)
	if relay.count() != 1 {
		t.Fatalf("the relay was handed %d messages, want exactly one", relay.count())
	}
	if strings.Contains(relay.bodies[0], moved) {
		t.Fatal("the former holder was mailed a task reassigned before the send")
	}
	if !strings.Contains(relay.bodies[0], kept) {
		t.Fatal("the work still theirs was not mailed")
	}
}

// A change committed while the reservation waits on its row is seen by the
// reservation. It then reads the work again instead of mailing the old body.
func TestAMorningReservationWaitsForAConcurrentChangeToItsWork(t *testing.T) {
	relay := &countingMailer{}
	b := setupBriefJob(t).withMailer(relay)
	morning := time.Date(2026, 6, 4, 9, 0, 0, 0, time.UTC)
	b.now = morning
	assignee := ids.From[ids.UserKind](b.Rep1)
	kept, moved := "Send the revised Globex quote", "Call Initech about the renewal"
	seedWorkFor(t, b.Env, morning, b.Rep1)
	task, _, err := b.Activities.LogActivity(b.Admin(), activities.LogActivityInput{Kind: "task", Subject: &moved, AssigneeID: &assignee, DueAt: &morning, Source: "manual"})
	if err != nil {
		t.Fatal(err)
	}
	ctx := b.As(b.Rep1, []ids.UUID{b.Team1}, integration.AdminPerms)
	candidate, ready, err := b.worker.prepareMorningMail(ctx, morning)
	if err != nil || !ready {
		t.Fatalf("the morning was not ready to send: %v", err)
	}
	held, release := make(chan struct{}), make(chan struct{})
	writer, written := make(chan error, 1), make(chan struct{})
	var holder int32
	go func() {
		defer close(written)
		writer <- database.WithWorkspaceTx(b.Admin(), b.Pool, func(tx pgx.Tx) error {
			if _, err := tx.Exec(context.Background(), `UPDATE activity SET assignee_id = $2 WHERE id = $1`, ids.UUID(task.Id), b.Rep2); err != nil {
				return err
			}
			if err := tx.QueryRow(context.Background(), `SELECT pg_backend_pid()`).Scan(&holder); err != nil {
				return err
			}
			close(held)
			<-release
			return nil
		})
	}()
	select {
	case <-held:
	case err := <-writer:
		t.Fatalf("the concurrent change ended before it held the work: %v", err)
	}
	sent := make(chan struct{})
	go func() {
		b.worker.sendMorning(ctx, candidate, morning)
		close(sent)
	}()
	var lift sync.Once
	t.Cleanup(func() {
		lift.Do(func() { close(release) })
		<-written
		<-sent
	})
	testdb.AwaitQueuedBehind(t, integration.OwnerConn(t), holder, 1, sent, "the morning reservation")
	lift.Do(func() { close(release) })
	if err := <-writer; err != nil {
		t.Fatal(err)
	}
	<-sent
	if relay.count() != 1 || strings.Contains(relay.bodies[0], moved) || !strings.Contains(relay.bodies[0], kept) {
		t.Fatalf("after a concurrent reassignment the relay got %d message(s): %v", relay.count(), relay.bodies)
	}
}

// Once the reservation commits, the message goes to the relay. A change made
// during the relay call cannot unsend it, and no second message follows.
func TestAChangeDuringTheRelayCallDoesNotSendTwice(t *testing.T) {
	relay := &countingMailer{}
	b := setupBriefJob(t).withMailer(relay)
	morning := time.Date(2026, 6, 4, 9, 0, 0, 0, time.UTC)
	b.now = morning
	assignee := ids.From[ids.UserKind](b.Rep1)
	title := "Call Initech about the renewal"
	task, _, err := b.Activities.LogActivity(b.Admin(), activities.LogActivityInput{Kind: "task", Subject: &title, AssigneeID: &assignee, DueAt: &morning, Source: "manual"})
	if err != nil {
		t.Fatal(err)
	}
	colleague := ids.From[ids.UserKind](b.Rep2)
	relay.onSend = func() {
		if _, err := b.Activities.UpdateActivity(b.Admin(), ids.From[ids.ActivityKind](ids.UUID(task.Id)), activities.UpdateActivityInput{AssigneeID: &colleague}); err != nil {
			t.Error(err)
		}
	}
	ctx := b.As(b.Rep1, []ids.UUID{b.Team1}, integration.AdminPerms)
	candidate, ready, err := b.worker.prepareMorningMail(ctx, morning)
	if err != nil || !ready {
		t.Fatalf("the morning was not ready to send: %v", err)
	}
	b.worker.sendMorning(ctx, candidate, morning)
	relay.onSend = nil
	b.worker.sendMorning(ctx, candidate, morning)
	if relay.count() != 1 || !strings.Contains(relay.bodies[0], title) {
		t.Fatalf("the relay got %d message(s), want the one reserved before the change", relay.count())
	}
}

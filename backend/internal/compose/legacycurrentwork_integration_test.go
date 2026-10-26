// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// Work an earlier pass derived from a mail's text stays current only while
// that text was read in full. Mail stored before the upgrade starts
// unassessed. Its old verdicts and admissions wait for the data task, and a
// member's own work stands on its own.

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// askedOfUs captures a customer's request to the seat and records the
// classifier's verdict on it, as a pass before the upgrade did.
func (o *owedEnv) askedOfUs(t *testing.T, key, body string) ids.UUID {
	t.Helper()
	message := o.capture(t, owedMail{
		from: "pat@customer.example", to: o.seat, subject: "Contract " + key, messageID: key + "@customer.example",
		body: body, encoding: "base64", authenticated: true, at: o.now.Add(-2 * time.Hour),
	})
	_, readAt, err := activities.NewStore(o.e.DB()).OwedBacklog(o.judge(), "prompts-test", o.now, 1, 400, 400)
	if err != nil {
		t.Fatal(err)
	}
	if applied, err := o.e.Activities.SetOwedVerdict(o.judge(), message, activities.OwedVerdictAsksUs, "prompts-test", readAt); err != nil || !applied {
		t.Fatalf("the verdict on %s: applied=%v err=%v", key, applied, err)
	}
	return message
}

func (o *owedEnv) captureRequests(t *testing.T) {
	t.Helper()
	ctx := principal.WithActor(o.e.Admin(), o.classifier())
	if err := activities.NewStore(o.e.DB()).CaptureEmailRequests(ctx, o.now); err != nil {
		t.Fatalf("CaptureEmailRequests: %v", err)
	}
}

func (o *owedEnv) requestTasks(t *testing.T, message ids.UUID) int {
	t.Helper()
	return o.e.WsCount(t, `SELECT count(*) FROM activity WHERE source_system = 'email_request' AND source_activity_id = $1`, message)
}

func (o *owedEnv) unlinkOriginal(t *testing.T, message ids.UUID) {
	t.Helper()
	if err := database.WithWorkspaceTx(o.e.Admin(), o.e.Pool, func(tx pgx.Tx) error {
		_, err := tx.Exec(context.Background(), `UPDATE activity SET raw_capture_id = NULL WHERE id = $1`, message)
		return err
	}); err != nil {
		t.Fatal(err)
	}
}

// The writer: a verdict from before the upgrade mints a request task only once
// its mail was checked and read in full, and only once.
func TestALegacyRequestBecomesATaskOnlyOnceItsMailIsReadInFull(t *testing.T) {
	o := setupOwed(t)
	o.now = clocktest.Now(t)
	o.contact(t, "Pat Buyer", "pat@customer.example")
	ask := "Could you send the signed contract?"
	assessable := o.askedOfUs(t, "assessable", readInFull(ask))
	partial := o.askedOfUs(t, "partial", brokenAfter(ask))
	noOriginal := o.askedOfUs(t, "no-original", readInFull(ask))
	complete := o.askedOfUs(t, "complete", readInFull(ask))
	o.asLegacy(t, assessable, "")
	o.asLegacy(t, partial, "")
	o.asLegacy(t, noOriginal, "")
	o.unlinkOriginal(t, noOriginal)

	o.captureRequests(t)
	for name, tc := range map[string]struct {
		message ids.UUID
		want    int
	}{"unassessed": {assessable, 0}, "partial": {partial, 0}, "no original": {noOriginal, 0}, "complete": {complete, 1}} {
		if got := o.requestTasks(t, tc.message); got != tc.want {
			t.Errorf("before the assessment, %s mail has %d request tasks, want %d", name, got, tc.want)
		}
	}

	o.runMailBodyTask(t)
	for range 2 {
		o.captureRequests(t)
	}
	for name, tc := range map[string]struct {
		message ids.UUID
		want    int
	}{"assessed whole": {assessable, 1}, "partial": {partial, 0}, "no original": {noOriginal, 0}, "complete": {complete, 1}} {
		if got := o.requestTasks(t, tc.message); got != tc.want {
			t.Errorf("after the assessment, %s mail has %d request tasks, want %d", name, got, tc.want)
		}
	}
}

// The readers: an automatic request task or promise task stays current only
// while its mail was read in full, and so does the needs-reply row. A task a
// member wrote stands on its own.
func TestCurrentWorkFromAMailNotReadInFullIsWithheld(t *testing.T) {
	o := setupOwed(t)
	o.now = clocktest.Now(t)
	o.contact(t, "Pat Buyer", "pat@customer.example")
	ask := "Could you send the signed contract?"
	request := o.askedOfUs(t, "current-request", readInFull(ask))
	control := o.askedOfUs(t, "current-control", readInFull(ask))
	o.captureRequests(t)
	// A request with an open request task is shown as that task, not as a
	// waiting row. These two have none, so the waiting lane shows them.
	waitingRequest := o.askedOfUs(t, "waiting-request", readInFull(ask))
	waitingControl := o.askedOfUs(t, "waiting-control", readInFull(ask))
	promise := "I will send the pricing by Friday."
	source := o.weWrite(t, owedMail{
		to: "pat@customer.example", subject: "Pricing", body: readInFull(promise),
		encoding: "base64", messageID: "current-promise@ws.example", at: o.now.Add(-time.Hour),
	})
	if got := o.promiseOutcome(t, source, promise); got != CommitmentTaskWritten {
		t.Fatalf("the promise fixture wrote %q, want a task", got)
	}
	reader := o.reader()
	own, _, err := o.e.Activities.LogActivity(reader, activities.LogActivityInput{
		Kind: "task", Subject: new("Call Pat back"), AssigneeID: new(ids.From[ids.UserKind](o.owner)),
	})
	if err != nil {
		t.Fatal(err)
	}

	openTasks := func() map[ids.UUID]bool {
		t.Helper()
		tasks, _, err := activities.NewStore(o.e.DB()).ListOpenTasks(reader, activities.ListOpenTasksInput{AsOf: o.now, AssigneeID: &o.owner})
		if err != nil {
			t.Fatal(err)
		}
		out := map[ids.UUID]bool{}
		for _, task := range tasks {
			out[task.ID] = true
		}
		return out
	}
	waiting := func() map[string]bool {
		t.Helper()
		feed := newAttentionService(o.e.Pool, approvals.NewService(o.e.DB()), func() time.Time { return o.now })
		page, err := feed.Worklist(reader, "mine", "all", ids.Nil, 100, "")
		if err != nil {
			t.Fatal(err)
		}
		out := map[string]bool{}
		for _, row := range page.Queue {
			if row.Source == "customer_waiting" {
				out[row.Id] = true
			}
		}
		return out
	}
	taskFor := func(sourceSystem string, message ids.UUID) ids.UUID {
		t.Helper()
		var id ids.UUID
		if err := database.WithWorkspaceTx(o.e.Admin(), o.e.Pool, func(tx pgx.Tx) error {
			return tx.QueryRow(context.Background(), `SELECT id FROM activity WHERE source_system = $1 AND source_activity_id = $2`,
				sourceSystem, message).Scan(&id)
		}); err != nil {
			t.Fatalf("the %s task for %v: %v", sourceSystem, message, err)
		}
		return id
	}
	requestTask, controlTask := taskFor("email_request", request), taskFor("email_request", control)
	promiseTask := taskFor(activities.CommitmentTaskSource, source)

	before, waitingBefore := openTasks(), waiting()
	for name, id := range map[string]ids.UUID{"request": requestTask, "control": controlTask, "promise": promiseTask, "own": ids.UUID(own.Id)} {
		if !before[id] {
			t.Fatalf("the %s task is not current work before the upgrade; the fixture proves nothing", name)
		}
	}
	if !waitingBefore[waitingRequest.String()] || !waitingBefore[waitingControl.String()] {
		t.Fatal("the requests are not waiting before the upgrade; the fixture proves nothing")
	}

	o.asLegacy(t, request, "")
	o.asLegacy(t, waitingRequest, "")
	o.asLegacy(t, source, "")
	after, waitingAfter := openTasks(), waiting()
	if after[requestTask] || after[promiseTask] {
		t.Errorf("automatic work from unassessed mail is still current: request %v, promise %v", after[requestTask], after[promiseTask])
	}
	if waitingAfter[waitingRequest.String()] {
		t.Error("a request from unassessed mail still waits on a reply")
	}
	if !after[controlTask] || !waitingAfter[waitingControl.String()] {
		t.Error("work from mail read in full stopped being current")
	}
	if !after[ids.UUID(own.Id)] {
		t.Error("the task a member wrote stopped being current")
	}
	// The record still holds the task; only its standing as current work waits.
	if n := o.e.WsCount(t, `SELECT count(*) FROM activity WHERE id = $1 AND archived_at IS NULL AND NOT is_done`, requestTask); n != 1 {
		t.Error("the withheld request task left the record")
	}
}

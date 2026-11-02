// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/modules/capture/mailmap"
	"github.com/margince/margince/backend/internal/modules/privacy"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/datatask"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/pipelinetrace"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestAFailedRepairUnitRetainsItsBarrierWithoutBlockingUnrelatedCapture(t *testing.T) {
	f := newThreadOwnerFixture(t)
	run := beginUnavailableThread(t, f.Env, f.source)
	apply := threadRepairApplier(f.Pool)
	actual := apply.Activity
	apply.Activity = func(ctx context.Context, tx pgx.Tx, id ids.ActivityID, from, to string) (bool, error) {
		changed, err := actual(ctx, tx, id, from, to)
		if err != nil || !changed {
			return changed, err
		}
		return false, errors.New("injected owner fault")
	}
	for attempt := 1; attempt <= 3; attempt++ {
		failed := false
		for range 1000 {
			if err := database.WithWorkspaceTx(f.Admin(), f.Pool, func(tx pgx.Tx) error {
				if _, _, _, err := capture.ThreadRepairBatchTx(f.Admin(), tx, 1, run, apply); err != nil {
					return err
				}
				return tx.QueryRow(f.Admin(), `SELECT EXISTS(SELECT 1 FROM capture_thread_repair WHERE failures=@attempt)`, pgx.NamedArgs{"attempt": attempt}).Scan(&failed)
			}); err != nil {
				t.Fatal(err)
			}
			if failed {
				break
			}
		}
		if !failed {
			t.Fatalf("attempt %d did not record its isolated fault", attempt)
		}
		if got := threadKeyOfMessage(t, f.Env, f.destination); got != f.source {
			t.Fatalf("failed owner move escaped savepoint: %s", got)
		}
		if attempt < 3 {
			if err := database.WithWorkspaceTx(f.Admin(), f.Pool, func(tx pgx.Tx) error {
				_, err := tx.Exec(f.Admin(), `UPDATE capture_thread_repair SET retry_after='2000-01-01' WHERE state='backoff'`)
				return err
			}); err != nil {
				t.Fatal(err)
			}
		}
	}
	if got := f.WsCount(t, `SELECT count(*) FROM capture_thread_repair WHERE state='quarantined' AND failures=3`); got != 1 {
		t.Fatalf("quarantined repairs=%d", got)
	}
	assertThreadRepairHealth(t, f.Env, 1)
	const other = "unrelated-after-fault@counterparty.example"
	captureThreadMail(t, f.Env, f.Rep1, subjectThreadMail(seatAddress(t, f.Env, f.Rep1), other, "Unrelated", ""))
	if got := threadKeyOfMessage(t, f.Env, other); got != other {
		t.Fatalf("unrelated capture=%s", got)
	}
	if err := database.WithWorkspaceTx(f.Admin(), f.Pool, func(tx pgx.Tx) error {
		var available bool
		if err := tx.QueryRow(f.Admin(), `SELECT activity_thread_available(@key)`, pgx.NamedArgs{"key": f.source}).Scan(&available); err != nil {
			return err
		}
		if available {
			t.Fatal("quarantined component became visible")
		}
		out, err := repairThreadsWithApplier(principal.SystemActing(f.Admin(), "system:test_thread_repair"), tx, datatask.BatchIn{Limit: 1, RunID: run}, threadRepairApplier(f.Pool))
		if out.Done {
			t.Fatalf("historical task must report quarantined component: %+v %v", out, err)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func TestAHiddenIdentityRemainsOwnedUntilCaptureCanRetry(t *testing.T) {
	o := setupOwed(t)
	id := o.customerWrites(t, "pat@customer.example", "Invoice", o.now.Add(-time.Hour))
	key := o.threadKey(t, id)
	run := beginUnavailableThread(t, o.e, key)
	ctx := threadConnectorCtx(o.e, o.owner)
	if err := database.WithWorkspaceTx(ctx, o.e.Pool, func(tx pgx.Tx) error {
		_, _, err := activities.ResolveBindableIdentity(ctx, tx, activities.IdentityKindMail, key)
		if !errors.Is(err, apperrors.ErrConflict) {
			t.Fatalf("hidden identity binding=%v", err)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	raw := owedMail{from: "pat@customer.example", to: o.seat, subject: "Invoice", messageID: key, at: o.now.Add(-time.Hour)}.raw()
	parsed, err := mailmap.Parse(raw, o.seat)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := newCaptureSink(o.e.Pool, CaptureConfig{}).Upsert(ctx, parsed.ToRecord("gmail", raw)); !errors.Is(err, apperrors.ErrConflict) {
		t.Fatalf("hidden replay=%v", err)
	}
	if n := o.count(t, `SELECT count(*) FROM activity_identity WHERE identity_kind='mail' AND identity_key=$1`, key); n != 1 {
		t.Fatalf("identity holders=%d", n)
	}
	finishUnavailableThread(t, o.e, run)
	if _, err := newCaptureSink(o.e.Pool, CaptureConfig{}).Upsert(ctx, parsed.ToRecord("gmail", raw)); err != nil {
		t.Fatal(err)
	}
}

func TestAnUnavailableOffThreadAnswerWithholdsUnansweredWork(t *testing.T) {
	o := setupOwed(t)
	o.contact(t, "Pat Buyer", "pat@customer.example")
	asked := o.customerWrites(t, "pat@customer.example", "Invoice March", o.now.Add(-3*time.Hour))
	reply := o.weWrite(t, owedMail{to: "pat@customer.example", subject: "Re: Invoice March", at: o.now.Add(-2 * time.Hour)})
	run := beginUnavailableThread(t, o.e, o.threadKey(t, reply))
	if err := database.WithWorkspaceTx(o.e.Admin(), o.e.Pool, func(tx pgx.Tx) error {
		var unanswered bool
		query := `SELECT ` + storekit.UnansweredSQL("a", "@until::timestamptz") + ` FROM activity a WHERE a.id=@id`
		if err := tx.QueryRow(o.e.Admin(), query, pgx.NamedArgs{"until": o.now, "id": asked}).Scan(&unanswered); err != nil {
			return err
		}
		if unanswered {
			t.Fatal("hidden answer revived unanswered work")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	finishUnavailableThread(t, o.e, run)
	if o.unanswered(t, asked) {
		t.Fatal("published answer did not settle the message")
	}
}

func TestAReplyCapturedDuringRepairIsEmittedOnceAfterPublication(t *testing.T) {
	o := setupOwed(t)
	const parent = "deferred-outbound@ws.example"
	o.weWrite(t, owedMail{to: "pat@customer.example", subject: "Invoice", messageID: parent, at: o.now.Add(-2 * time.Hour)})
	run := beginUnavailableThread(t, o.e, parent)
	child := o.capture(t, owedMail{from: "pat@customer.example", to: o.seat, subject: "Re: Invoice", messageID: "deferred-inbound@customer.example", inReplyTo: parent, at: o.now.Add(-time.Hour)})
	const replies = `SELECT count(*) FROM event_outbox WHERE envelope->>'type'='engagement.reply' AND envelope->'entity'->>'id'=$1`
	if n := o.count(t, replies, child.String()); n != 0 {
		t.Fatalf("premature reply events=%d", n)
	}
	finishUnavailableThread(t, o.e, run)
	if n := o.count(t, replies, child.String()); n != 1 {
		t.Fatalf("published reply events=%d", n)
	}
	o.capture(t, owedMail{from: "pat@customer.example", to: o.seat, subject: "Re: Invoice", messageID: "deferred-inbound@customer.example", inReplyTo: parent, at: o.now.Add(-time.Hour)})
	if n := o.count(t, replies, child.String()); n != 1 {
		t.Fatalf("replayed reply events=%d", n)
	}
	t.Run("an older parent arriving later still proves the reply", func(t *testing.T) {
		const lateParent = "late-outbound@ws.example"
		lateChild := o.capture(t, owedMail{
			from: "pat@customer.example", to: o.seat,
			subject: "Re: Quote", messageID: "early-inbound@customer.example", inReplyTo: lateParent,
			at: o.now.Add(-3 * time.Hour),
		})
		if n := o.count(t, replies, lateChild.String()); n != 0 {
			t.Fatalf("reply before its parent arrived=%d", n)
		}
		o.weWrite(t, owedMail{
			to: "pat@customer.example", subject: "Quote", messageID: lateParent,
			at: o.now.Add(-3 * time.Hour),
		})
		if n := o.count(t, replies, lateChild.String()); n != 1 {
			t.Fatalf("late equal-time parent reply events=%d", n)
		}
		o.capture(t, owedMail{
			from: "pat@customer.example", to: o.seat, subject: "Re: Quote",
			messageID: "early-inbound@customer.example", inReplyTo: lateParent, at: o.now.Add(-3 * time.Hour),
		})
		if n := o.count(t, replies, lateChild.String()); n != 1 {
			t.Fatalf("late-parent replay events=%d", n)
		}
	})
	t.Run("equal timestamps without ancestry do not prove a prior send", func(t *testing.T) {
		const firstKey = "coarse-inbound@customer.example"
		first := o.capture(t, owedMail{
			from: "pat@customer.example", to: o.seat,
			subject: "Coarse clock", messageID: firstKey, at: o.now.Add(-5 * time.Hour),
		})
		later := o.weWrite(t, owedMail{
			to: "pat@customer.example", subject: "Re: Coarse clock",
			messageID: "coarse-outbound@ws.example", inReplyTo: firstKey, at: o.now.Add(-5 * time.Hour),
		})
		o.e.WsExec(t, `UPDATE activity SET created_at=$1 WHERE id=$2 OR id=$3`, o.now, first, later)
		run := beginUnavailableThread(t, o.e, firstKey)
		finishUnavailableThread(t, o.e, run)
		if n := o.count(t, replies, first.String()); n != 0 {
			t.Fatalf("ambiguous later outbound manufactured reply events=%d", n)
		}
	})
}

func TestReferencesWithoutRecordedOrderNeverChooseALexicalParent(t *testing.T) {
	o := setupOwed(t)
	first := o.customerWrites(t, "pat@customer.example", "Invoice", o.now.Add(-3*time.Hour))
	second := o.customerWrites(t, "pat@customer.example", "Invoice", o.now.Add(-2*time.Hour))
	child := o.customerWrites(t, "pat@customer.example", "Invoice", o.now.Add(-time.Hour))
	source := o.threadKey(t, child)
	if err := database.WithWorkspaceTx(o.e.Admin(), o.e.Pool, func(tx pgx.Tx) error {
		ctx := o.e.Admin()
		if err := activities.RecordMailReferencesTx(ctx, tx, ids.From[ids.ActivityKind](child), []string{o.threadKey(t, first), o.threadKey(t, second)}); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `UPDATE activity_mail_reference SET reference_order=0 WHERE activity_id=@id`, pgx.NamedArgs{"id": child})
		return err
	}); err != nil {
		t.Fatal(err)
	}
	run := beginUnavailableThread(t, o.e, source)
	finishUnavailableThread(t, o.e, run)
	if got := o.threadKey(t, child); got != source {
		t.Fatalf("unordered refs selected parent %s", got)
	}
}

func TestProvenProjectTagsSurviveAnotherSeatsReplayAndProjectArchive(t *testing.T) {
	o := setupOwed(t)
	o.e.GrantRole(t, o.owner, "admin")
	account := seedLadderAccount(t, o.e)
	project, key := account.project(t, "Stable correspondence tag")
	const parent = "project-proof-parent@customer.example"
	const child = "project-proof-child@customer.example"
	captureThreadMail(t, o.e, o.owner, subjectThreadMail(o.seat, parent, "Price", ""))
	raw := subjectThreadMail(o.seat, child, "["+key+"] Price", parent)
	other := seatAddress(t, o.e, o.e.Rep2)
	raw = []byte(strings.Replace(string(raw), "Subject:", "Cc: "+other+"\r\nSubject:", 1))
	parsed, err := mailmap.Parse(raw, o.seat)
	if err != nil {
		t.Fatal(err)
	}
	ctx := threadConnectorCtx(o.e, o.owner)
	actor, ok := principal.Actor(ctx)
	if !ok {
		t.Fatal("capturing seat missing")
	}
	actor.Permissions.Objects["project"] = principal.ObjectGrant{Read: true}
	ctx = principal.WithActor(ctx, actor)
	if _, err := newCaptureSink(o.e.Pool, CaptureConfig{}).Upsert(ctx, parsed.ToRecord("gmail", raw)); err != nil {
		t.Fatal(err)
	}
	finishThreadRepairs(t, o.e)
	if got := threadKeyOfMessage(t, o.e, child); got != parent {
		t.Fatalf("proven project tag root=%s", got)
	}
	captureThreadMail(t, o.e, o.e.Rep2, raw)
	finishThreadRepairs(t, o.e)
	if got := threadKeyOfMessage(t, o.e, child); got != parent {
		t.Fatalf("less-authorized seat changed root=%s", got)
	}
	if _, err := o.e.Projects.ArchiveProject(o.e.Admin(), ids.From[ids.ProjectKind](project), nil); err != nil {
		t.Fatal(err)
	}
	captureThreadMail(t, o.e, o.owner, raw)
	finishThreadRepairs(t, o.e)
	if got := threadKeyOfMessage(t, o.e, child); got != parent {
		t.Fatalf("archiving proven project changed root=%s", got)
	}
}

func TestAnOpaqueImportedKeyCannotAbsorbAnUnrelatedMessageRoot(t *testing.T) {
	o := setupOwed(t)
	const key = "real-root-collision@customer.example"
	original := o.capture(t, owedMail{from: "pat@customer.example", to: o.seat, subject: "Price", messageID: key, at: o.now.Add(-time.Hour)})
	subject, body, system, external, direction := "Unrelated importer thread", "Imported text", "hubspot", "opaque-source", "inbound"
	logged, _, err := o.e.Activities.LogActivity(o.reader(), activities.LogActivityInput{Kind: "email", Subject: &subject, Body: &body, Source: "hubspot_import:1", SourceSystem: &system, SourceID: &external, Direction: &direction, ThreadKey: key, RFCMessageID: "opaque-own-message@customer.example", EmailFrom: "other@customer.example", EmailTo: []string{o.seat}})
	if err != nil {
		t.Fatal(err)
	}
	run := beginUnavailableThread(t, o.e, key)
	finishUnavailableThread(t, o.e, run)
	if got := o.threadKey(t, original); got != key {
		t.Fatalf("real identity lost root=%s", got)
	}
	if got := o.threadKey(t, ids.UUID(logged.Id)); got == key {
		t.Fatalf("opaque importer key still shares unrelated message root=%s", got)
	}
}

func TestThreadRepairWithholdsTracePayloadsAndGovernedAuditImages(t *testing.T) {
	o := setupOwed(t)
	message := o.customerWrites(t, "pat@customer.example", "Invoice", o.now.Add(-time.Hour))
	id := ids.From[ids.ActivityKind](message)
	key := o.threadKey(t, message)
	subject := "Reviewed invoice"
	if _, err := o.e.Activities.UpdateActivity(o.reader(), id, activities.UpdateActivityInput{Subject: &subject}); err != nil {
		t.Fatal(err)
	}
	if err := database.WithWorkspaceTx(o.e.Admin(), o.e.Pool, func(tx pgx.Tx) error {
		return capture.Trace(o.e.Admin(), tx, capture.TraceEntry{Stage: pipelinetrace.StageTierLadder, UserID: o.owner, Connector: "gmail", SourceSystem: "email", SourceID: key, Outcome: capture.TraceCaptured, ActivityID: message, Counterparty: "pat@customer.example", Subject: subject}, true)
	}); err != nil {
		t.Fatal(err)
	}
	trace := capture.NewTraceStore(o.e.DB())
	before, err := trace.LadderByActivityID(o.reader(), message, true)
	if err != nil || before.ActivityID == nil || len(before.Rungs) == 0 {
		t.Fatalf("readable trace=%+v: %v", before, err)
	}
	run := beginUnavailableThread(t, o.e, key)
	hidden, err := trace.LadderByActivityID(o.reader(), message, true)
	if err != nil {
		t.Fatal(err)
	}
	if hidden.ActivityID != nil {
		t.Fatal("hidden ladder retained activity link")
	}
	for _, rung := range hidden.Rungs {
		if rung.ActivityID != nil || rung.Subject != "" || rung.Counterparty != "" {
			t.Fatalf("hidden trace leaked content: %+v", rung)
		}
	}
	entity := "activity"
	page, err := privacy.ListAuditLog(o.e.Admin(), o.e.DB(), privacy.AuditFilter{EntityType: &entity, EntityID: &message})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Entries) == 0 {
		t.Fatal("compliance envelope disappeared")
	}
	for _, entry := range page.Entries {
		if bytes.Contains(entry.Before, []byte(subject)) || bytes.Contains(entry.After, []byte(subject)) {
			t.Fatalf("hidden activity audit retained images: %s %s", entry.Before, entry.After)
		}
	}
	finishUnavailableThread(t, o.e, run)
	published, err := trace.LadderByActivityID(o.reader(), message, true)
	if err != nil || published.ActivityID == nil {
		t.Fatalf("published ladder=%+v: %v", published, err)
	}
}

func TestCorrectionRetryRearmsOneQuarantinedConversation(t *testing.T) {
	o := setupOwed(t)
	id := o.customerWrites(t, "pat@customer.example", "Invoice", o.now.Add(-time.Hour))
	key := o.threadKey(t, id)
	beginUnavailableThread(t, o.e, key)
	if err := database.WithWorkspaceTx(o.e.Admin(), o.e.Pool, func(tx pgx.Tx) error {
		_, err := tx.Exec(o.e.Admin(), `UPDATE capture_thread_repair SET state='quarantined',failures=3`)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	assertThreadRepairHealth(t, o.e, 1)
	task := threadSplitDataTask(o.e.Pool)
	task.Requires = nil
	task.Steps = task.Steps[2:]
	now := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	notices := 0
	engine, err := datatask.New(o.e.Pool, []datatask.Task{task}, "test", func() time.Time { return now }, func(context.Context, pgx.Tx, datatask.Progress) error { notices++; return nil })
	if err != nil {
		t.Fatal(err)
	}
	ctx := principal.SystemActing(o.e.Admin(), "system:test_thread_correction")
	if err := engine.Seed(ctx); err != nil {
		t.Fatal(err)
	}
	for range 6 {
		if _, err := engine.Execute(ctx); err != nil {
			t.Fatal(err)
		}
		now = now.Add(24 * time.Hour)
	}
	progress, err := engine.List(dataTaskReader(o.e))
	if err != nil || len(progress) != 1 || progress[0].Status != datatask.Failed || notices != 1 {
		t.Fatalf("quarantine progress=%+v notices=%d: %v", progress, notices, err)
	}
	if err := engine.Control(dataTaskReader(o.e), task.ID, task.Steps[0].Name, "retry", progress[0].Version); err != nil {
		t.Fatal(err)
	}
	assertThreadRepairHealth(t, o.e, 0)
	if n := o.count(t, `SELECT count(*) FROM capture_thread_repair WHERE state='backoff' AND failures=0`); n != 1 {
		t.Fatalf("rearmed components=%d", n)
	}
	for range 100 {
		if _, err := engine.Execute(ctx); err != nil {
			t.Fatal(err)
		}
		progress, err = engine.List(dataTaskReader(o.e))
		if err != nil {
			t.Fatal(err)
		}
		if progress[0].Status == datatask.Done {
			return
		}
	}
	t.Fatalf("retried correction did not finish: %+v", progress)
}

func TestContradictoryDirectParentsNeverChooseALexicalWinner(t *testing.T) {
	o := setupOwed(t)
	first := o.customerWrites(t, "pat@customer.example", "Invoice", o.now.Add(-3*time.Hour))
	second := o.customerWrites(t, "pat@customer.example", "Invoice", o.now.Add(-2*time.Hour))
	child := o.customerWrites(t, "pat@customer.example", "Invoice", o.now.Add(-time.Hour))
	source := o.threadKey(t, child)
	references := []string{o.threadKey(t, first), o.threadKey(t, second)}
	if err := database.WithWorkspaceTx(o.e.Admin(), o.e.Pool, func(tx pgx.Tx) error {
		for _, parent := range references {
			if err := activities.RecordMailReferencesTx(o.e.Admin(), tx, ids.From[ids.ActivityKind](child), []string{parent}); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	run := beginUnavailableThread(t, o.e, source)
	finishUnavailableThread(t, o.e, run)
	if got := o.threadKey(t, child); got != source {
		t.Fatalf("conflicting direct parents selected %s", got)
	}
}

func TestAnUnavailableCompletedReminderDoesNotReopenItsRequest(t *testing.T) {
	o := setupOwed(t)
	o.contact(t, "Buyer", "buyer@customer.example")
	source := o.customerWrites(t, "buyer@customer.example", "Please send the report", o.now.Add(-time.Hour))
	if _, err := o.e.Activities.SetOwedVerdict(o.judge(), source, activities.OwedVerdictAsksUs, "test", o.now); err != nil {
		t.Fatal(err)
	}
	system, external, title := activities.EmailRequestTaskSource, "settled-reminder", "Send the report"
	const key = "hidden-completed-reminder"
	task, _, err := o.e.Activities.LogActivity(principal.SystemActing(o.reader(), "system:email_request"), activities.LogActivityInput{Kind: "task", Subject: &title, Source: system, SourceSystem: &system, SourceID: &external, SourceActivityID: &source, ThreadKey: key})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := o.e.Activities.UpdateActivity(o.reader(), ids.From[ids.ActivityKind](ids.UUID(task.Id)), activities.UpdateActivityInput{IsDone: new(true)}); err != nil {
		t.Fatal(err)
	}
	if o.badged(t, source) {
		t.Fatal("completed reminder left request open")
	}
	run := beginUnavailableThread(t, o.e, key)
	if o.badged(t, source) {
		t.Fatal("hidden completed reminder reopened request")
	}
	finishUnavailableThread(t, o.e, run)
	if o.badged(t, source) {
		t.Fatal("published completed reminder reopened request")
	}
}

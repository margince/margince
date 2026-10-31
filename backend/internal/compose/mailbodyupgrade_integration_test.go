// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// Mail stored before body completeness was recorded. The older parser stopped
// at the first part it could not decode and stored only the From/To lines.
// Such a body must not count as complete after the upgrade. The data task
// assesses it from its stored original, through the parser capture uses today.

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/datatask"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// asLegacy puts a captured mail back in the state the upgrade finds it in. It
// is unassessed, and holds the body the older parser stored when lostText is set.
func (o *owedEnv) asLegacy(t *testing.T, message ids.UUID, lostText string) {
	t.Helper()
	if err := database.WithWorkspaceTx(o.e.Admin(), o.e.Pool, func(tx pgx.Tx) error {
		_, err := tx.Exec(context.Background(), `UPDATE activity
			SET body_completeness = 'unassessed', body = coalesce(NULLIF($2, ''), body)
			WHERE id = $1 AND raw_capture_id IS NOT NULL`, message, lostText)
		return err
	}); err != nil {
		t.Fatalf("making the mail legacy: %v", err)
	}
	if got := o.completeness(t, message); got != "unassessed" {
		t.Fatalf("the legacy fixture has no stored original (completeness %q)", got)
	}
}

func (o *owedEnv) promiseOutcome(t *testing.T, source ids.UUID, promise string) CommitmentOutcome {
	t.Helper()
	c := Commitment{
		Meaning: openCommitmentMeaning("", promise), AsOf: o.now, SourceActivityID: source,
		ReadMessages: []ids.UUID{source}, Extractor: signalScanActor, Seat: &o.owner,
		Summary: "Send the pricing", Quote: promise, Body: promise, Confidence: 0.95,
		Locator: commitmentLocator(source, claimKindOurs, o.owner.String(), promise),
	}
	ctx := o.judge()
	dispatcher := NewCommitmentDispatcher(o.e.Pool, approvals.NewService(o.e.DB()))
	var outcome CommitmentOutcome
	if err := database.WithWorkspaceTx(ctx, o.e.Pool, func(tx pgx.Tx) error {
		var err error
		outcome, _, err = dispatcher.DispatchTx(ctx, tx, c, ids.NewV7())
		return err
	}); err != nil {
		t.Fatal(err)
	}
	return outcome
}

func (o *owedEnv) offeredAsRequest(t *testing.T, message ids.UUID) bool {
	t.Helper()
	backlog, _, err := activities.NewStore(o.e.DB()).OwedBacklog(o.judge(), "prompts-test", o.now, 100, 400, 400)
	if err != nil {
		t.Fatalf("reading the owed backlog: %v", err)
	}
	for _, candidate := range backlog {
		if candidate.ID == message {
			return true
		}
	}
	return false
}

// runMailBodyTask runs the completeness task to its end through the executor.
func (o *owedEnv) runMailBodyTask(t *testing.T) datatask.Task {
	t.Helper()
	ctx := principal.SystemActing(o.e.Admin(), "system:datatask-test")
	now := func() time.Time { return o.now }
	task := registeredMailBodyTask(t, o)
	engine, err := datatask.New(o.e.Pool, []datatask.Task{task}, "test", now, func(context.Context, pgx.Tx, datatask.Progress) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	if err := engine.Seed(ctx); err != nil {
		t.Fatal(err)
	}
	for range 40 {
		if _, err := engine.Execute(ctx); err != nil {
			t.Fatal(err)
		}
		progress, err := engine.List(ctx)
		if err != nil {
			t.Fatal(err)
		}
		done := 0
		for _, step := range progress {
			if step.Status == datatask.Done {
				done++
			}
		}
		if done == len(task.Steps) {
			return task
		}
	}
	t.Fatal("the completeness task did not finish")
	return datatask.Task{}
}

// registeredMailBodyTask is the task as the worker registers it, with small
// batches so paging is exercised.
func registeredMailBodyTask(t *testing.T, o *owedEnv) datatask.Task {
	t.Helper()
	for _, task := range dataTaskCatalog(o.e.Pool, func() time.Time { return o.now }) {
		if task.ID == mailBodyCompletenessDataTask().ID {
			for i := range task.Steps {
				task.Steps[i].Limit = 2
			}
			return task
		}
	}
	t.Fatal("the completeness task is not registered")
	return datatask.Task{}
}

func TestMailStoredBeforeTheUpgradeIsNotTrustedUntilItsOriginalSaysSo(t *testing.T) {
	o := setupOwed(t)
	o.now = clocktest.Now(t)
	o.contact(t, "Pat Buyer", "pat@customer.example")
	promise := "I will send the pricing by Friday."
	ask := "Could you send the signed NDA?"

	brokenPromise := o.weWrite(t, owedMail{
		to: "pat@customer.example", subject: "Pricing", body: brokenAfter(promise), encoding: "base64",
		at: o.now.Add(-2 * time.Hour),
	})
	wholePromise := o.weWrite(t, owedMail{
		to: "pat@customer.example", subject: "Pricing again", body: readInFull(promise), encoding: "base64",
		at: o.now.Add(-2 * time.Hour),
	})
	brokenAsk := o.capture(t, owedMail{
		from: "pat@customer.example", to: o.seat, subject: "NDA", body: brokenAfter(ask), encoding: "base64",
		messageID: "legacy-ask@customer.example", at: o.now.Add(-time.Hour),
	})
	wholeAsk := o.capture(t, owedMail{
		from: "pat@customer.example", to: o.seat, subject: "NDA again", body: readInFull(ask), encoding: "base64",
		messageID: "legacy-ask-2@customer.example", at: o.now.Add(-time.Hour),
	})
	o.asLegacy(t, brokenPromise, "From: "+o.seat+"\nTo: pat@customer.example")
	o.asLegacy(t, wholePromise, "")
	o.asLegacy(t, brokenAsk, "From: pat@customer.example\nTo: "+o.seat)
	o.asLegacy(t, wholeAsk, "")
	adopted, _, err := o.e.Activities.LogActivity(o.e.Admin(), activities.LogActivityInput{
		Kind: "task", Subject: new("Send Pat the pricing"), Body: new("Adopted by hand."),
		SourceActivityID: &brokenPromise,
	})
	if err != nil {
		t.Fatal(err)
	}
	adoptedBefore, _ := o.e.Activities.GetActivity(o.e.Admin(), ids.From[ids.ActivityKind](ids.UUID(adopted.Id)), storekit.LiveOnly)

	// Before the task runs, legacy mail waits: nothing is read from it.
	if got := o.promiseOutcome(t, wholePromise, promise); got != CommitmentOutcome(activities.CommitmentSourceUnassessed) {
		t.Fatalf("an unassessed promise was admitted as %q", got)
	}
	if o.offeredAsRequest(t, wholeAsk) {
		t.Fatal("an unassessed request was offered for judgement")
	}

	task := o.runMailBodyTask(t)
	for message, want := range map[ids.UUID]string{
		brokenPromise: "partial", wholePromise: "complete", brokenAsk: "partial", wholeAsk: "complete",
	} {
		if got := o.completeness(t, message); got != want {
			t.Errorf("body_completeness of %v = %q after the task, want %q", message, got, want)
		}
	}
	if got := o.promiseOutcome(t, brokenPromise, promise); got != CommitmentOutcome(activities.CommitmentSourceIncomplete) {
		t.Errorf("a promise whose text an older parser lost was admitted as %q", got)
	}
	if got := o.promiseOutcome(t, wholePromise, promise); got != CommitmentTaskWritten {
		t.Errorf("a whole legacy promise was admitted as %q, want a task", got)
	}
	if o.offeredAsRequest(t, brokenAsk) {
		t.Error("a request whose text an older parser lost was offered for judgement")
	}
	if !o.offeredAsRequest(t, wholeAsk) {
		t.Error("a whole legacy request was not offered for judgement")
	}
	adoptedAfter, err := o.e.Activities.GetActivity(o.e.Admin(), ids.From[ids.ActivityKind](ids.UUID(adopted.Id)), storekit.LiveOnly)
	if err != nil || adoptedAfter.Version == nil || adoptedBefore.Version == nil || *adoptedAfter.Version != *adoptedBefore.Version {
		t.Errorf("the task a member adopted changed: before %v, after %v (%v)", adoptedBefore.Version, adoptedAfter.Version, err)
	}
	assertMailBodyReplayChangesNothing(t, o, task)
}

// A second run from the start finds nothing left to assess.
func assertMailBodyReplayChangesNothing(t *testing.T, o *owedEnv, task datatask.Task) {
	t.Helper()
	ctx := principal.SystemActing(o.e.Admin(), "system:datatask-test")
	for _, step := range task.Steps {
		in := datatask.BatchIn{Limit: 2, RunID: ids.NewV7()}
		for {
			var out datatask.BatchOut
			if err := database.WithWorkspaceTx(ctx, o.e.Pool, func(tx pgx.Tx) error {
				var err error
				out, err = step.Batch(ctx, tx, in)
				return err
			}); err != nil {
				t.Fatal(err)
			}
			if out.Changed != 0 {
				t.Fatalf("a second run of %s changed %d rows", step.Name, out.Changed)
			}
			if out.Done {
				break
			}
			in.Cursor = out.Cursor
		}
	}
}

// Captured mail whose stored original is missing or no longer decodes is never
// guessed at. Nothing is left to check it against, so it is unverifiable, and a
// second run leaves it so. Mail a member logged is their own whole text.
func TestMailWithoutAReadableOriginalIsUnverifiable(t *testing.T) {
	o := setupOwed(t)
	o.now = clocktest.Now(t)
	var captured, undecodable, logged ids.UUID
	if err := database.WithWorkspaceTx(o.e.Admin(), o.e.Pool, func(tx pgx.Tx) error {
		insert := `INSERT INTO activity (kind, subject, body, direction, occurred_at, source, captured_by, body_completeness)
			VALUES ('email', 'hi', 'From: a@x.test\nTo: b@y.test', 'inbound', $1, $2, $3, 'unassessed') RETURNING id`
		if err := tx.QueryRow(context.Background(), insert, o.now, "gmail:seed", "connector:gmail").Scan(&captured); err != nil {
			return err
		}
		return tx.QueryRow(context.Background(), insert, o.now, "manual", "human:"+o.owner.String()).Scan(&logged)
	}); err != nil {
		t.Fatal(err)
	}
	if err := integration.OwnerConn(t).QueryRow(context.Background(), `
		WITH rc AS (INSERT INTO raw_capture (source_system, source_id, payload)
		            VALUES ('email', 'garbled@x.test', '{"encoding": "base64", "data": "%%% not base64"}'::jsonb) RETURNING id)
		INSERT INTO activity (kind, subject, body, direction, occurred_at, source, captured_by, raw_capture_id, body_completeness)
		SELECT 'email', 'hi', 'From: a@x.test' || chr(10) || 'To: b@y.test', 'inbound', $1, 'gmail:seed', 'connector:gmail', id, 'unassessed'
		  FROM rc RETURNING id`, o.now).Scan(&undecodable); err != nil {
		t.Fatal(err)
	}
	task := o.runMailBodyTask(t)
	for name, message := range map[string]ids.UUID{"with no original": captured, "with an original that no longer decodes": undecodable} {
		if got := o.completeness(t, message); got != "unverifiable" {
			t.Errorf("captured mail %s reads %q, want unverifiable", name, got)
		}
	}
	if got := o.completeness(t, logged); got != "complete" {
		t.Errorf("logged mail was assessed as %q, want complete", got)
	}
	assertMailBodyReplayChangesNothing(t, o, task)
}

// The task writes nothing a caller other than the system may ask for, and
// nothing over a body that changed after it was read.
func TestTheCompletenessWriterRefusesAStaleBodyAndAMember(t *testing.T) {
	o := setupOwed(t)
	o.now = clocktest.Now(t)
	message := o.weWrite(t, owedMail{to: "pat@customer.example", subject: "Pricing", body: "Text.", at: o.now.Add(-time.Hour)})
	o.asLegacy(t, message, "")
	system := principal.SystemActing(o.e.Admin(), "system:datatask-test")
	stale := []activities.MailBodyAssessment{{Activity: message, Completeness: "complete", Body: "an older body"}}
	if err := database.WithWorkspaceTx(system, o.e.Pool, func(tx pgx.Tx) error {
		changed, err := activities.AssessMailBodiesTx(system, tx, stale)
		if err == nil && changed != 0 {
			t.Errorf("an assessment of a body that changed since was written (%d rows)", changed)
		}
		return err
	}); err != nil {
		t.Fatal(err)
	}
	member := o.e.Admin()
	if err := database.WithWorkspaceTx(member, o.e.Pool, func(tx pgx.Tx) error {
		_, err := assessMailBodiesBatch(member, tx, datatask.BatchIn{Limit: 2, RunID: ids.NewV7()})
		return err
	}); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Errorf("a member ran the completeness batch: err = %v, want permission denied", err)
	}
	if got := o.completeness(t, message); got != "unassessed" {
		t.Errorf("body_completeness = %q, want it left unassessed", got)
	}
}

// A restricted mail refuses every update and is not offered. An unlinked mail
// is not matched to an original by its Message-ID.
func TestTheCompletenessTaskSkipsRestrictedAndAmbiguousMail(t *testing.T) {
	o := setupOwed(t)
	o.now = clocktest.Now(t)
	restricted := o.weWrite(t, owedMail{to: "pat@customer.example", subject: "Held", body: "Text.", at: o.now.Add(-time.Hour)})
	o.asLegacy(t, restricted, "")
	owner := integration.OwnerConn(t)
	ctx := context.Background()
	// The whole shape a restriction takes; the database refuses a partial one.
	if _, err := owner.Exec(ctx, `
		INSERT INTO activity_retention_evidence (activity_id, basis, qualified_at, decided_by_name, reason)
		VALUES ($1, 'controller_pin', now(), 'Datenschutz', 'litigation hold')`, restricted); err != nil {
		t.Fatal(err)
	}
	if _, err := owner.Exec(ctx, `
		UPDATE activity SET restricted_at = now(), archived_at = now(), restricted_reason = 'litigation hold',
		       restricted_until = now() + interval '1 year',
		       retention_class = 'commercial_correspondence', retention_class_at = now()
		 WHERE id = $1`, restricted); err != nil {
		t.Fatal(err)
	}
	var ambiguous ids.UUID
	if err := owner.QueryRow(ctx, `
		WITH rc AS (INSERT INTO raw_capture (source_system, source_id, payload)
		            VALUES ('email', 'claimed@x.test', to_jsonb('From: a@x.test'::text)) RETURNING id),
		     claimant AS (INSERT INTO activity (kind, subject, body, direction, occurred_at, source_system, source_id,
		                                        source, captured_by, raw_capture_id)
		                  SELECT 'email', 'other', 'From: a@x.test', 'inbound', now(), 'gmail', 'other@x.test',
		                         'gmail:seed', 'connector:gmail', id FROM rc)
		INSERT INTO activity (kind, subject, body, direction, occurred_at, source_system, source_id, source, captured_by, body_completeness)
		VALUES ('email', 'hi', 'From: a@x.test' || chr(10) || 'To: -', 'inbound', now(), 'email', 'claimed@x.test',
		        'gmail:seed', 'connector:gmail', 'unassessed')
		RETURNING id`).Scan(&ambiguous); err != nil {
		t.Fatal(err)
	}
	o.runMailBodyTask(t)
	if got := o.completeness(t, restricted); got != "unassessed" {
		t.Errorf("restricted mail was assessed as %q", got)
	}
	// Unlinked, so it has no original of its own: unverifiable, and never
	// judged from the original its Message-ID would find.
	if got := o.completeness(t, ambiguous); got != "unverifiable" {
		t.Errorf("unlinked mail reads %q, want unverifiable", got)
	}
}

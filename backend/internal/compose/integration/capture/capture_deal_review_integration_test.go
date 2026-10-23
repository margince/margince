// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package capture

// The deal rule's shared writers under concurrency, the whole conversation
// population, the bulk answer, the repair's honest outcomes and its relink
// event. Every writer is the production one; races are set up by what the
// database reports, never by sleeping.

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"runtime"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/activities"
	capturemod "github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	kevents "github.com/margince/margince/backend/internal/shared/kernel/events"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// holdInHumanTx runs fn in a colleague's transaction and keeps it open until
// release is closed. It answers the transaction's backend pid once fn ran.
func holdInHumanTx(t *testing.T, e *integration.SearchEnv, w dealWorld, fn func(tx pgx.Tx) error, release <-chan struct{}) (<-chan uint32, <-chan error) {
	t.Helper()
	pid, done := make(chan uint32, 1), make(chan error, 1)
	go func() {
		done <- database.WithWorkspaceTx(w.ctx, e.Pool, func(tx pgx.Tx) error {
			var backend uint32
			if err := tx.QueryRow(context.Background(), `SELECT pg_backend_pid()`).Scan(&backend); err != nil {
				return err
			}
			if err := fn(tx); err != nil {
				return err
			}
			pid <- backend
			<-release
			return nil
		})
	}()
	return pid, done
}

func relinkInTx(w dealWorld, tx pgx.Tx, mail ids.ActivityID, deal ids.UUID, replace bool) error {
	_, err := activities.RelinkActivityInTx(w.ctx, tx, mail, activities.RelinkActivityInput{
		EntityType: "deal", EntityID: deal, ReplaceExistingOfType: replace,
	})
	return err
}

// A colleague files a sibling on a deal of another company while another
// colleague's answer waits at that sibling's conditional write. The holder
// takes only the sibling's row lock before the answer starts, so the one
// thing the answer can wait on is that row: the answer has filed its own mail
// and chosen the sibling by then. The sibling keeps the deal it was filed on,
// with no second deal and no audit row from the answer.
func TestAnAnswerKeepsASiblingFiledWhileItWaited(t *testing.T) {
	env := newCaptureEnv(t)
	e := env.e
	w := newDealWorld(t, e)
	w.employee(t, "alice@acme.example", w.customer)
	w.deal(t, "Acme operations")
	reporting := w.deal(t, "Acme reporting")
	elsewhere := w.at(t, "Elsewhere").deal(t, "Elsewhere")
	env.syncSent(t, map[string]bool{"race2a@myco.example": true},
		email(captureOwner, "", "alice@acme.example", "race2a@myco.example", ""))
	env.sync(t, mailFrom("alice@acme.example", "race2b@acme.example", "race2a@myco.example", "Re: project"))
	if n := openQuestions(t, e); n != 1 {
		t.Fatalf("%d open questions, want 1", n)
	}
	sibling := ids.From[ids.ActivityKind](activityOf(t, e, "race2a@myco.example"))
	answered := ids.From[ids.ActivityKind](activityOf(t, e, "race2b@acme.example"))

	lockedRow, fileNow, release := make(chan struct{}), make(chan struct{}), make(chan struct{})
	pid, held := holdInHumanTx(t, e, w, func(tx pgx.Tx) error {
		if _, err := tx.Exec(context.Background(), `SELECT id FROM activity WHERE id = $1 FOR UPDATE`, sibling); err != nil {
			return err
		}
		close(lockedRow)
		<-fileNow
		return relinkInTx(w, tx, sibling, elsewhere, false)
	}, release)
	<-lockedRow
	answer := make(chan error, 1)
	go func() {
		_, err := activities.NewStore(e.DB()).RelinkActivity(w.ctx, answered, activities.RelinkActivityInput{EntityType: "deal", EntityID: reporting})
		answer <- err
	}()
	// The answer reaches the sibling's row while the holder has written
	// nothing; then the holder files the sibling and commits.
	waitForActivityRowLock(t, e)
	close(fileNow)
	<-pid
	close(release)
	if err := <-held; err != nil {
		t.Fatalf("the colleague's filing: %v", err)
	}
	if err := <-answer; err != nil {
		t.Fatalf("the answer: %v", err)
	}
	wantDeals(t, e, "race2a@myco.example", elsewhere)
	wantDeals(t, e, "race2b@acme.example", reporting)
	if n := countRows(t, e, `SELECT count(*) FROM audit_log WHERE action = 'activity_relink'
		 AND entity_id = (SELECT id FROM activity WHERE source_id = 'race2a@myco.example')`); n != 1 {
		t.Fatalf("the sibling has %d relink audit rows, want only the colleague's", n)
	}
	if n := openQuestions(t, e); n != 0 {
		t.Fatalf("%d questions open after the answer, want 0", n)
	}
}

// waitForActivityRowLock returns once a session waits in an activity row
// lock statement, as pg_stat_activity reports it.
func waitForActivityRowLock(t *testing.T, e *integration.SearchEnv) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	for {
		var waiting int
		if err := e.Pool.QueryRow(ctx, `SELECT count(*) FROM pg_stat_activity
			 WHERE datname = current_database() AND wait_event_type = 'Lock'
			   AND query LIKE '%activity%FOR UPDATE%'`).Scan(&waiting); err != nil {
			t.Fatalf("reading who waits on the row: %v", err)
		}
		if waiting >= 1 {
			return
		}
		runtime.Gosched()
	}
}

// The conversation is one correspondent's. Our mail to alice filed on one
// deal is no clue for bob's reply on the same References root; the company's
// two deals leave bob's mail on none. Siblings filed on two of alice's
// company's deals make her authenticated reply ambiguous, though a third
// company's deal sorts between them.
func TestTheThreadRungReadsOneWholeConversation(t *testing.T) {
	env := newCaptureEnv(t)
	e := env.e
	w := newDealWorld(t, e)
	w.employee(t, "alice@acme.example", w.customer)
	w.employee(t, "bob@acme.example", w.customer)
	first := w.deal(t, "Acme one")
	middle := w.at(t, "Between").deal(t, "Between")
	second := w.deal(t, "Acme two")
	// Filed through the in-transaction door, which answers no question, so
	// the conversation's siblings stay where each was put.
	file := func(sourceID string, deal ids.UUID) {
		t.Helper()
		mail := ids.From[ids.ActivityKind](activityOf(t, e, sourceID))
		if err := database.WithWorkspaceTx(w.ctx, e.Pool, func(tx pgx.Tx) error { return relinkInTx(w, tx, mail, deal, true) }); err != nil {
			t.Fatalf("filing %s: %v", sourceID, err)
		}
	}
	env.syncSent(t, map[string]bool{"one1@myco.example": true, "one2@myco.example": true, "one3@myco.example": true},
		email(captureOwner, "", "alice@acme.example", "one1@myco.example", ""),
		email(captureOwner, "", "alice@acme.example", "one2@myco.example", "one1@myco.example"),
		email(captureOwner, "", "alice@acme.example", "one3@myco.example", "one1@myco.example"))
	file("one1@myco.example", first)
	file("one2@myco.example", middle)

	env.sync(t, authenticated(mailFrom("bob@acme.example", "bob1@acme.example", "one1@myco.example", "Re: project"), "acme.example"))
	wantDeals(t, e, "bob1@acme.example")
	env.sync(t, authenticated(mailFrom("alice@acme.example", "ali1@acme.example", "one1@myco.example", "Re: project"), "acme.example"))
	wantDeals(t, e, "ali1@acme.example", first)

	file("one3@myco.example", second)
	env.sync(t, authenticated(mailFrom("alice@acme.example", "ali2@acme.example", "one1@myco.example", "Re: project"), "acme.example"))
	wantDeals(t, e, "ali2@acme.example")
}

// A bulk filing that names mail of two questioned conversations answers both,
// and a repeated id is one row.
func TestABulkFilingAnswersEveryConversationItNames(t *testing.T) {
	env := newCaptureEnv(t)
	e := env.e
	w := newDealWorld(t, e)
	w.employee(t, "alice@acme.example", w.customer)
	w.employee(t, "carol@acme.example", w.customer)
	w.deal(t, "Acme operations")
	reporting := w.deal(t, "Acme reporting")
	env.syncSent(t, map[string]bool{"bulk1@myco.example": true, "bulk3@myco.example": true},
		email(captureOwner, "", "alice@acme.example", "bulk1@myco.example", ""),
		email(captureOwner, "", "carol@acme.example", "bulk3@myco.example", ""))
	env.sync(t,
		mailFrom("alice@acme.example", "bulk2@acme.example", "bulk1@myco.example", "Re: project"),
		mailFrom("carol@acme.example", "bulk4@acme.example", "bulk3@myco.example", "Re: project"),
		mailFrom("alice@acme.example", "lone@acme.example", "", "unrelated"))
	if n := openQuestions(t, e); n != 2 {
		t.Fatalf("%d open questions, want one per conversation", n)
	}
	lone, second, fourth := activityOf(t, e, "lone@acme.example"), activityOf(t, e, "bulk2@acme.example"), activityOf(t, e, "bulk4@acme.example")
	if _, err := activities.NewStore(e.DB()).RelinkActivities(w.ctx, []ids.UUID{lone, second, fourth, second},
		activities.RelinkActivityInput{EntityType: "deal", EntityID: reporting}); err != nil {
		t.Fatalf("filing in bulk: %v", err)
	}
	for _, sourceID := range []string{"bulk1@myco.example", "bulk2@acme.example", "bulk3@myco.example", "bulk4@acme.example"} {
		wantDeals(t, e, sourceID, reporting)
	}
	if n := openQuestions(t, e); n != 0 {
		t.Fatalf("%d questions open after the bulk answer, want 0", n)
	}
}

// A colleague files the mail elsewhere and back while the repair waits for
// its lock. Filed back to the same deal is still a colleague's choice: the
// repair keeps it, and moves nothing.
func TestTheRepairKeepsAColleaguesFilingMadeWhileItWaited(t *testing.T) {
	env := newCaptureEnv(t)
	e := env.e
	w := newDealWorld(t, e)
	alice := w.employee(t, "alice@acme.example", w.customer)
	first := w.deal(t, "Acme rollout")
	env.sync(t, mailFrom("alice@acme.example", "aba1@acme.example", "", "status"))
	wantDeals(t, e, "aba1@acme.example", first)
	second := w.deal(t, "Acme reporting")
	w.stakeholder(t, alice, second)
	elsewhere := w.at(t, "Elsewhere").deal(t, "Elsewhere")
	mail := ids.From[ids.ActivityKind](activityOf(t, e, "aba1@acme.example"))

	release := make(chan struct{})
	pid, held := holdInHumanTx(t, e, w, func(tx pgx.Tx) error {
		if err := relinkInTx(w, tx, mail, elsewhere, true); err != nil {
			return err
		}
		return relinkInTx(w, tx, mail, first, true)
	}, release)
	holder := <-pid
	r := newRepairer(e, w)
	type result struct {
		changed int
		err     error
	}
	repaired := make(chan result, 1)
	go func() {
		changed, err := r.runErr()
		repaired <- result{changed, err}
	}()
	waitForLockWaiters(t, e, holder, 1)
	close(release)
	if err := <-held; err != nil {
		t.Fatalf("the colleague's filings: %v", err)
	}
	if got := <-repaired; got.err != nil || got.changed != 0 {
		t.Fatalf("the repair changed %d mails (err %v), want none", got.changed, got.err)
	}
	wantDeals(t, e, "aba1@acme.example", first)
}

// The rule's new answer is a deal a colleague closes while the repair waits
// for its row. The repair asks the deal's admission before it takes anything
// back, so it reports the refusal: the rule's old link comes off and nothing
// is written in its place. The change is announced with the relink event
// its consumers read.
func TestTheRepairReportsARefusedTargetAndAnnouncesItsChange(t *testing.T) {
	env := newCaptureEnv(t)
	e := env.e
	w := newDealWorld(t, e)
	alice := w.employee(t, "alice@acme.example", w.customer)
	first := w.deal(t, "Acme rollout")
	env.sync(t, mailFrom("alice@acme.example", "ref1@acme.example", "", "status"))
	wantDeals(t, e, "ref1@acme.example", first)
	closing := w.deal(t, "Acme reporting")
	w.stakeholder(t, alice, closing)
	var lost ids.UUID
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(), `INSERT INTO stage (pipeline_id, name, position, semantic, win_probability)
			VALUES ($1, 'Lost', 9, 'lost', 0) RETURNING id`, w.pipelineID).Scan(&lost)
	}); err != nil {
		t.Fatalf("seeding the lost stage: %v", err)
	}

	release := make(chan struct{})
	reason := "Budget moved"
	pid, held := holdInHumanTx(t, e, w, func(tx pgx.Tx) error {
		_, err := w.store.AdvanceDealTx(w.ctx, tx, ids.From[ids.DealKind](closing), deals.AdvanceDealInput{
			ToStageID: ids.From[ids.StageKind](lost), LostReason: &reason,
		}, deals.CustomColumns{})
		return err
	}, release)
	holder := <-pid
	r := newRepairer(e, w)
	type result struct {
		changed int
		err     error
	}
	repaired := make(chan result, 1)
	go func() {
		changed, err := r.runErr()
		repaired <- result{changed, err}
	}()
	waitForLockWaiters(t, e, holder, 1)
	close(release)
	if err := <-held; err != nil {
		t.Fatalf("closing the deal: %v", err)
	}
	if got := <-repaired; got.err != nil || got.changed != 1 {
		t.Fatalf("the repair changed %d mails (err %v), want 1", got.changed, got.err)
	}
	wantDeals(t, e, "ref1@acme.example")
	if n := countRows(t, e, `SELECT count(*) FROM audit_log WHERE action = 'activity_relink' AND actor_type = 'system'
		 AND evidence->>'reason' = 'captured_mail_deal_repair' AND before->>'entity_id' = '`+first.String()+`'`); n != 1 {
		t.Fatalf("%d repair retraction audit rows, want 1", n)
	}
	if n := countRows(t, e, `SELECT count(*) FROM event_outbox o
		 WHERE o.envelope->>'type' = 'activity.updated'
		   AND o.envelope->'payload'->'changed_fields'->'relinked'->>'entity_id' = '`+first.String()+`'`); n != 2 {
		t.Fatalf("%d relink events name the old deal, want capture's filing and the repair's retraction", n)
	}
}

// What the repair cannot resolve stays visible to the operator after it ran.
func TestTheOperatorPreviewStillShowsWhatTheRepairLeft(t *testing.T) {
	env := newCaptureEnv(t)
	e := env.e
	w := newDealWorld(t, e)
	w.employee(t, "alice@acme.example", w.customer)
	only := w.deal(t, "Acme rollout")
	env.sync(t, mailFrom("alice@acme.example", "left1@acme.example", "", "status"))
	env.syncFiledUnderDeal(t, map[string][]ids.UUID{"left2@acme.example": {only}},
		mailFrom("alice@acme.example", "left2@acme.example", "", "other"))
	r := newRepairer(e, w)
	r.run(t)
	preview := r.preview(t)
	if preview[capturemod.DealUnknownLink] != 1 || preview[capturemod.DealMachineConfirmed] != 1 {
		t.Fatalf("after the repair the operator preview reads %v, want the unknown link and the confirmed one", preview)
	}
}

// Two unrelated mails whose thread key is empty are no conversation: our
// filed mail is no clue for the other, and the company's two deals leave it
// on none.
func TestMailWithAnEmptyThreadKeyFollowsNoConversation(t *testing.T) {
	env := newCaptureEnv(t)
	e := env.e
	w := newDealWorld(t, e)
	w.employee(t, "alice@acme.example", w.customer)
	env.syncSent(t, map[string]bool{"empty1@myco.example": true},
		email(captureOwner, "", "alice@acme.example", "empty1@myco.example", ""))
	env.sync(t, authenticated(mailFrom("alice@acme.example", "empty2@acme.example", "", "other"), "acme.example"))
	first := w.deal(t, "Acme operations")
	w.deal(t, "Acme reporting")
	ours := ids.From[ids.ActivityKind](activityOf(t, e, "empty1@myco.example"))
	if err := database.WithWorkspaceTx(w.ctx, e.Pool, func(tx pgx.Tx) error { return relinkInTx(w, tx, ours, first, false) }); err != nil {
		t.Fatal(err)
	}
	// A row stored with an empty key, as an import or an older writer may
	// leave one; no live writer is asked to produce it.
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		_, err := tx.Exec(context.Background(), `UPDATE activity SET thread_key = '' WHERE source_id IN ('empty1@myco.example', 'empty2@acme.example')`)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	newRepairer(e, w).run(t)
	wantDeals(t, e, "empty2@acme.example")
}

// The operator preview reads a bounded page: a limit outside 1..1000 is
// refused, and the repair's own preview refuses one below 1.
func TestTheOperatorPreviewRefusesALimitOutOfBounds(t *testing.T) {
	env := newCaptureEnv(t)
	e := env.e
	w := newDealWorld(t, e)
	r := newRepairer(e, w)
	for _, body := range []string{`{"limit":0}`, `{"limit":1001}`} {
		if rec := r.operatorPreview(body); rec.Code != http.StatusUnprocessableEntity {
			t.Errorf("a preview of %s answered %d, want 422", body, rec.Code)
		}
	}
	if rec := r.operatorPreview(`{"limit":1}`); rec.Code != http.StatusOK {
		t.Errorf("a preview of one mail answered %d, want 200", rec.Code)
	}
	if err := database.WithWorkspaceTx(r.system, e.Pool, func(tx pgx.Tx) error {
		_, err := r.repair.PreviewTx(r.system, tx, ids.Nil, 0)
		return err
	}); !errors.Is(err, apperrors.ErrInvalidArgument) {
		t.Errorf("a repair preview of no mails answered %v, want invalid argument", err)
	}
}

// readingQueue is the stage-evidence reading queue at its true boundary: it
// records each reading the trigger asks for.
type readingQueue struct {
	asked []compose.StageEvidenceReadArgs
}

func (q *readingQueue) EnqueueTx(_ context.Context, _ pgx.Tx, args river.JobArgs, _ *river.InsertOpts) error {
	if reading, ok := args.(compose.StageEvidenceReadArgs); ok {
		q.asked = append(q.asked, reading)
	}
	return nil
}

// The repair's link reaches the stage-evidence trigger as the relink event
// it reads: a current event queues one reading of the mail against its deal,
// a redelivery is the same reading, and the same event replayed past the
// trigger's backlog window queues nothing.
func TestTheRepairsLinkIsReadByTheStageEvidenceTrigger(t *testing.T) {
	env := newCaptureEnv(t)
	e := env.e
	w := newDealWorld(t, e)
	w.employee(t, "alice@acme.example", w.customer)
	env.sync(t, mailFrom("alice@acme.example", "ev1@acme.example", "", "status"))
	deal := w.deal(t, "Acme rollout")
	if changed := newRepairer(e, w).run(t); changed != 1 {
		t.Fatalf("the repair changed %d mails, want 1", changed)
	}
	mail := activityOf(t, e, "ev1@acme.example")
	var raw []byte
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(), `SELECT envelope FROM event_outbox
			 WHERE envelope->>'type' = 'activity.updated' AND envelope->'entity'->>'id' = $1
			   AND envelope->'payload'->'changed_fields'->'relinked'->>'entity_id' = $2`, mail.String(), deal.String()).Scan(&raw)
	}); err != nil {
		t.Fatalf("reading the repair's relink event: %v", err)
	}
	var envelope kevents.Envelope
	if err := json.Unmarshal(raw, &envelope); err != nil {
		t.Fatalf("decoding the repair's relink event: %v", err)
	}
	queue := &readingQueue{}
	trigger := compose.NewStageEvidenceTrigger(e.Pool, compose.StageEvidenceDeals(e.Pool), compose.StageEvidenceDomains(e.Pool),
		queue, nil, slog.Default())
	for range 2 {
		if err := trigger.HandleEvent(context.Background(), envelope); err != nil {
			t.Fatalf("delivering the relink event: %v", err)
		}
	}
	if len(queue.asked) == 0 || queue.asked[0].DealID != deal || queue.asked[0].ActivityID != mail {
		t.Fatalf("the trigger asked for %+v, want a reading of the mail against its deal", queue.asked)
	}
	if len(queue.asked) == 2 && queue.asked[0] != queue.asked[1] {
		t.Fatalf("a redelivery asked for a different reading: %+v", queue.asked)
	}
	old := envelope
	old.OccurredAt = time.Now().Add(-365 * 24 * time.Hour)
	queue.asked = nil
	if err := trigger.HandleEvent(context.Background(), old); err != nil {
		t.Fatalf("delivering the replayed event: %v", err)
	}
	if len(queue.asked) != 0 {
		t.Fatalf("a replayed old event queued %d readings, want none", len(queue.asked))
	}
}

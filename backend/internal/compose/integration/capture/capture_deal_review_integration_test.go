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
	"errors"
	"net/http"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/activities"
	capturemod "github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/apperrors"
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

// A colleague files a sibling on one deal while another colleague's answer
// waits for that sibling's lock. The sibling keeps the deal it was filed on,
// with no second deal and no audit row from the answer.
func TestAnAnswerKeepsASiblingFiledWhileItWaited(t *testing.T) {
	env := newCaptureEnv(t)
	e := env.e
	w := newDealWorld(t, e)
	w.employee(t, "alice@acme.example", w.customer)
	operations := w.deal(t, "Acme operations")
	reporting := w.deal(t, "Acme reporting")
	env.syncSent(t, map[string]bool{"race2a@myco.example": true},
		email(captureOwner, "", "alice@acme.example", "race2a@myco.example", ""))
	env.sync(t, mailFrom("alice@acme.example", "race2b@acme.example", "race2a@myco.example", "Re: project"))
	if n := openQuestions(t, e); n != 1 {
		t.Fatalf("%d open questions, want 1", n)
	}
	sibling := ids.From[ids.ActivityKind](activityOf(t, e, "race2a@myco.example"))
	answered := ids.From[ids.ActivityKind](activityOf(t, e, "race2b@acme.example"))

	release := make(chan struct{})
	pid, held := holdInHumanTx(t, e, w, func(tx pgx.Tx) error { return relinkInTx(w, tx, sibling, operations, false) }, release)
	holder := <-pid
	answer := make(chan error, 1)
	go func() {
		_, err := activities.NewStore(e.DB()).RelinkActivity(w.ctx, answered, activities.RelinkActivityInput{EntityType: "deal", EntityID: reporting})
		answer <- err
	}()
	waitForLockWaiters(t, e, holder, 1)
	close(release)
	if err := <-held; err != nil {
		t.Fatalf("the colleague's filing: %v", err)
	}
	if err := <-answer; err != nil {
		t.Fatalf("the answer: %v", err)
	}
	wantDeals(t, e, "race2a@myco.example", operations)
	wantDeals(t, e, "race2b@acme.example", reporting)
	if n := countRows(t, e, `SELECT count(*) FROM audit_log WHERE action = 'activity_relink'
		 AND entity_id = (SELECT id FROM activity WHERE source_id = 'race2a@myco.example') AND actor_type = 'human'`); n != 1 {
		t.Fatalf("the sibling has %d relink audit rows, want only the colleague's", n)
	}
	if n := openQuestions(t, e); n != 0 {
		t.Fatalf("%d questions open after the answer, want 0", n)
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
		   AND o.envelope->'payload'->'changed_fields'->'relinked'->>'entity_id' = '`+first.String()+`'`); n != 1 {
		t.Fatalf("the repair's change wrote %d relink events, want the one its consumers read", n)
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

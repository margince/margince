// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// The stakeholder mail backfill: mail of a deal's stakeholder that capture
// kept off the deal, and the silent close that missing mail caused. Mail
// arrives through the production sink and the pass runs as the refile job.

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"
	"github.com/riverqueue/river/rivertest"
	"github.com/riverqueue/river/rivertype"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/jobs"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/model"
)

const stakeholderAddress = "carol@consultancy.example"

// backfillEnv is the close-lost world with a mailbox for its owner and a
// contact at a firm that has no deal of its own.
type backfillEnv struct {
	*closeLostEnv
	o     *owedEnv
	carol ids.UUID
}

func setupBackfill(t *testing.T) *backfillEnv {
	t.Helper()
	c := setupCloseLost(t)
	integration.ApplyRiverSchema(t)
	o := &owedEnv{e: c.Env, owner: c.Rep1, seat: seatAddress(t, c.Env, c.Rep1), now: c.now}
	err := database.WithTx(c.Admin(), c.Pool, func(tx pgx.Tx) error {
		_, err := tx.Exec(context.Background(), `UPDATE capture_connection SET mail_posture = 'shared' WHERE user_id = $1`, o.owner)
		return err
	})
	if err != nil {
		t.Fatalf("sharing the mailbox: %v", err)
	}
	return &backfillEnv{closeLostEnv: c, o: o, carol: o.contact(t, "Carol", stakeholderAddress)}
}

// writes captures one authenticated mail from the stakeholder, daysAgo before
// the test's clock. Capture finds no seat yet, so the mail stays off every deal.
func (b *backfillEnv) writes(t *testing.T, daysAgo int) ids.UUID {
	t.Helper()
	return b.o.capture(t, owedMail{
		from: stakeholderAddress, to: b.o.seat, subject: "Rollout plan", at: b.now.AddDate(0, 0, -daysAgo),
		messageID: "bf-" + ids.NewV7().String() + "@consultancy.example", authenticated: true,
	})
}

// seat makes the contact a stakeholder on the deal, as a colleague would.
func (b *backfillEnv) seat(t *testing.T, contact, deal ids.UUID) {
	t.Helper()
	member, target := ids.From[ids.ContactKind](contact), ids.From[ids.DealKind](deal)
	if _, err := b.Contacts.CreateRelationship(b.Admin(), contacts.CreateRelationshipInput{
		Kind: "deal_stakeholder", ContactID: &member, DealID: &target, Source: "manual",
	}); err != nil {
		t.Fatal(err)
	}
}

// pass runs the recovery sweep's pass to its end, as the hourly job does.
func (b *backfillEnv) pass(t *testing.T, backfill bool) {
	t.Helper()
	b.passPaged(t, backfill, capture.DealRefileLimit, nil)
}

// passFails runs the pass the way the hourly job does and reports whether the
// job failed, as it does while the model is down. A failed job leaves the
// stored position for the next pass.
func (b *backfillEnv) passFails(t *testing.T, brain completer) bool {
	t.Helper()
	ctx := principal.WithCorrelationID(principal.SystemActing(b.Admin(), dealRefileActor), ids.NewV7())
	var args CapturedDealRefileArgs
	err := b.DB().Tx(ctx, func(tx pgx.Tx) error {
		var err error
		args, err = dealRefilePassArgs(ctx, tx, capture.DealRefileScope{All: true, Recheck: true, Backfill: true}, b.now, ids.Nil)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	spec, _ := jobs.SpecFor(args.Kind())
	worker := rivertest.NewWorker(t, riverpgxv5.New(b.Pool), &river.Config{}, jobs.Govern(newCapturedDealRefileWorker(b.Pool, brain), spec))
	var row *rivertype.JobRow
	for range 10 {
		tx, err := b.Pool.Begin(t.Context())
		if err != nil {
			t.Fatal(err)
		}
		var result *rivertest.WorkResult
		if row == nil {
			result, err = worker.Work(boundBackground(t), t, tx, args, nil)
		} else {
			result, err = worker.WorkJob(t.Context(), t, tx, row)
		}
		if commit := tx.Commit(t.Context()); commit != nil {
			t.Fatal(commit)
		}
		if err != nil || result.EventKind == river.EventKindJobFailed {
			return true
		}
		if result.EventKind == river.EventKindJobCompleted {
			return false
		}
		row = result.Job
	}
	t.Fatal("the pass neither finished nor failed")
	return false
}

// flakyTopic is the topic model with an outage switch.
type flakyTopic struct{ down bool }

func (f *flakyTopic) Complete(ctx context.Context, req model.Request) (model.Response, error) {
	if f.down {
		return model.Response{}, errors.New("provider outage")
	}
	return aboutEveryDeal.Complete(ctx, req)
}

// passPaged is pass with a worker that inspects page candidates per attempt,
// over the topic model given.
func (b *backfillEnv) passPaged(t *testing.T, backfill bool, page int, brain completer) {
	t.Helper()
	ctx := principal.WithCorrelationID(principal.SystemActing(b.Admin(), dealRefileActor), ids.NewV7())
	var args CapturedDealRefileArgs
	err := b.DB().Tx(ctx, func(tx pgx.Tx) error {
		var err error
		args, err = dealRefilePassArgs(ctx, tx, capture.DealRefileScope{All: true, Recheck: true, Backfill: backfill}, b.now, ids.Nil)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	dealRefileFixture{o: b.o, topic: brain}.runPaged(t, args, page)
}

func (b *backfillEnv) filedOn(t *testing.T, mail, deal ids.UUID) bool {
	t.Helper()
	return b.WsCount(t, `SELECT count(*) FROM activity_link WHERE activity_id = $1 AND deal_id = $2`, mail, deal) == 1
}

func (b *backfillEnv) finished(t *testing.T) bool {
	t.Helper()
	var done bool
	err := database.WithTx(b.Admin(), b.Pool, func(tx pgx.Tx) error {
		var err error
		done, err = capture.StakeholderBackfillDoneTx(b.Admin(), tx)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return done
}

// A stakeholder's mail older than the waiting horizon is on no deal, and only
// the backfill files it, and only inside the silent close window.
func TestTheBackfillFilesAStakeholdersOldMailOnTheOpenDeal(t *testing.T) {
	b := setupBackfill(t)
	within := b.writes(t, 60)
	beyond := b.writes(t, 150)
	b.seat(t, b.carol, b.silent)
	b.setBackfillFinished(t, false)

	b.pass(t, false)
	if b.filedOn(t, within, b.silent) {
		t.Fatal("the plain refile pass filed mail older than the waiting horizon")
	}
	before := b.WsScalar(t, `SELECT coalesce(last_activity_at::text, '') FROM deal WHERE id = $1`, b.silent)

	b.pass(t, true)
	if !b.filedOn(t, within, b.silent) {
		t.Fatal("the backfill left the stakeholder's mail of 60 days ago off the deal they sit on")
	}
	if b.filedOn(t, beyond, b.silent) {
		t.Fatal("the backfill filed mail older than the silent close window")
	}
	if after := b.WsScalar(t, `SELECT coalesce(last_activity_at::text, '') FROM deal WHERE id = $1`, b.silent); after == before {
		t.Error("filing the mail did not move the deal's last activity")
	}
	if !b.finished(t) {
		t.Error("the backfill is not recorded as finished")
	}
}

// Proof decides, as at capture: a Cc, an unauthenticated From and a consumer
// mail provider's From each file nothing.
func TestTheBackfillFilesNothingWithoutProof(t *testing.T) {
	b := setupBackfill(t)
	cc := b.o.capture(t, owedMail{
		from: "eve@stranger.example", to: b.o.seat, cc: stakeholderAddress, subject: "Rollout plan",
		at: b.now.AddDate(0, 0, -60), messageID: "bf-cc@stranger.example", authenticated: true,
	})
	unproven := b.o.capture(t, owedMail{
		from: stakeholderAddress, to: b.o.seat, subject: "Rollout plan", at: b.now.AddDate(0, 0, -60),
		messageID: "bf-unproven@consultancy.example",
	})
	b.seat(t, b.carol, b.silent)
	b.setBackfillFinished(t, false)

	b.pass(t, true)
	for name, mail := range map[string]ids.UUID{"a Cc": cc, "an unauthenticated From": unproven} {
		if b.filedOn(t, mail, b.silent) {
			t.Errorf("the backfill filed mail proven only by %s", name)
		}
	}
}

// A close for silence that rested on mail the deal never had comes back to its
// owner past the Home window. The mail now sits on the lost deal. Undo is the
// existing one, and the next check keeps the reopened deal open.
func TestTheBackfillOffersBackASilentCloseThatMissedMail(t *testing.T) {
	b := setupBackfill(t)
	missed := b.writes(t, 40)
	b.seat(t, b.carol, b.silent)
	if got := b.run(t); got != 1 || b.status(t, b.silent) != "lost" {
		t.Fatalf("setup: closed %d, deal is %s; want the silent deal lost", got, b.status(t, b.silent))
	}
	b.WsExec(t, `UPDATE deal_correction SET applied_at = $2 WHERE deal_id = $1`, b.silent, b.now.AddDate(0, 0, -10))
	if listed := b.pending(t); len(listed) != 0 {
		t.Fatalf("a close from ten days ago is on Home before the backfill: %v", listed)
	}
	b.setBackfillFinished(t, false)

	b.pass(t, true)
	if !b.filedOn(t, missed, b.silent) {
		t.Fatal("the missed mail is not filed on the lost deal")
	}
	if listed := b.pending(t); len(listed) != 1 || listed[0] != b.silent {
		t.Fatalf("Home lists %v after the backfill, want the offered-back close of the silent deal", listed)
	}
	if n := b.WsCount(t, `SELECT count(*) FROM activity a JOIN activity_link l ON l.activity_id = a.id
		WHERE l.deal_id = $1 AND a.kind = 'note' AND a.subject LIKE 'Margince found mail%'`, b.silent); n != 1 {
		t.Errorf("timeline notes naming why the close is back = %d, want 1", n)
	}

	b.undo(t, b.silent)
	if got := b.status(t, b.silent); got != "open" {
		t.Fatalf("after Undo the deal is %s, want open", got)
	}
	if got := b.run(t); got != 0 {
		t.Fatalf("the next check closed %d deals; the filed mail should keep the reopened deal open", got)
	}
}

// A close that was right, one the owner undid, and one with no missed mail
// are not offered back.
func TestTheBackfillOffersBackNothingThatWasRight(t *testing.T) {
	b := setupBackfill(t)
	b.seat(t, b.carol, b.silent)
	b.run(t)
	b.WsExec(t, `UPDATE deal_correction SET applied_at = $2 WHERE deal_id = $1`, b.silent, b.now.AddDate(0, 0, -10))
	b.setBackfillFinished(t, false)

	b.pass(t, true)
	if n := b.WsCount(t, `SELECT count(*) FROM deal_correction WHERE deal_id = $1 AND reoffered_at IS NOT NULL`, b.silent); n != 0 {
		t.Fatalf("a close with no missed mail was offered back %d times", n)
	}
	if !b.finished(t) {
		t.Error("the backfill is not recorded as finished")
	}
}

// While the backfill has not run, no deal closes for silence, and a refusal
// still closes.
func TestNoDealClosesForSilenceBeforeTheBackfillRan(t *testing.T) {
	b := setupBackfill(t)
	b.setBackfillFinished(t, false)
	b.refuses(t, b.supported, b.now.AddDate(0, 0, -2))

	if got := b.run(t); got != 1 {
		t.Fatalf("closed %d deals, want only the refused one", got)
	}
	if got := b.status(t, b.silent); got != "open" {
		t.Fatalf("the silent deal is %s before the backfill ran, want open", got)
	}
	if got := b.status(t, b.supported); got != "lost" {
		t.Fatalf("the refused deal is %s, want lost", got)
	}
}

// pending lists the deals on the owner's Home changes.
func (b *backfillEnv) pending(t *testing.T) []ids.UUID {
	t.Helper()
	summary, err := b.Deals.PendingChangesOwnedBy(b.owner(t), nil)
	if err != nil {
		t.Fatal(err)
	}
	var listed []ids.UUID
	for _, bundle := range summary.Bundles {
		listed = append(listed, bundle.DealID.UUID)
	}
	return listed
}

// A pass that stops after every mail resumes from the stored position, so the
// model is asked once per mail and every mail is filed.
func TestTheBackfillResumesWithoutAskingTwice(t *testing.T) {
	b := setupBackfill(t)
	first := b.writes(t, 60)
	second := b.writes(t, 70)
	b.seat(t, b.carol, b.silent)
	b.setBackfillFinished(t, false)
	brain := &countingTopicModel{}

	b.passPaged(t, true, 1, brain)
	for _, mail := range []ids.UUID{first, second} {
		if !b.filedOn(t, mail, b.silent) {
			t.Errorf("mail %s is not filed after a pass that paged one mail at a time", mail)
		}
	}
	if brain.calls != 2 {
		t.Errorf("the topic check was asked %d times for two mails, want 2", brain.calls)
	}
	if !b.finished(t) {
		t.Error("the backfill is not recorded as finished")
	}
}

// A workspace with no activity has a zero ceiling. The backfill still runs its
// phases and finishes, so the nightly silent close is not held for good.
func TestTheBackfillFinishesInAWorkspaceWithNoActivity(t *testing.T) {
	o := setupOwed(t)
	integration.ApplyRiverSchema(t)
	ctx := principal.WithCorrelationID(principal.SystemActing(o.e.Admin(), dealRefileActor), ids.NewV7())
	var args CapturedDealRefileArgs
	if err := o.e.DB().Tx(ctx, func(tx pgx.Tx) error {
		var err error
		args, err = dealRefilePassArgs(ctx, tx, capture.DealRefileScope{All: true, Recheck: true, Backfill: true}, o.now, ids.Nil)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if !args.Ceiling.IsZero() {
		t.Fatal("setup: the workspace has live activity")
	}
	dealRefileFixture{o: o}.run(t, args)
	done := false
	if err := database.WithTx(o.e.Admin(), o.e.Pool, func(tx pgx.Tx) error {
		var err error
		done, err = capture.StakeholderBackfillDoneTx(o.e.Admin(), tx)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if !done {
		t.Fatal("the backfill never finished in a workspace with no activity")
	}
}

// A model outage fails the pass without moving its position or finishing it.
// The next pass, with the model back, files the mail and finishes.
func TestAnOutageInTheOpenPhaseIsRetriedNotSkipped(t *testing.T) {
	b := setupBackfill(t)
	mail := b.writes(t, 60)
	b.seat(t, b.carol, b.silent)
	b.setBackfillFinished(t, false)
	brain := &flakyTopic{down: true}

	if !b.passFails(t, brain) {
		t.Fatal("the pass did not fail while the model was down")
	}
	if b.filedOn(t, mail, b.silent) || b.finished(t) {
		t.Fatal("an outage filed the mail or finished the backfill")
	}
	if got := b.WsScalar(t, `SELECT value->>'open_after' FROM installation_state WHERE key = 'stakeholder_mail_backfill'`); got != ids.Nil.String() {
		t.Fatalf("the position moved to %s past a mail nobody judged", got)
	}
	brain.down = false
	if b.passFails(t, brain) {
		t.Fatal("the pass failed after the model came back")
	}
	if !b.filedOn(t, mail, b.silent) || !b.finished(t) {
		t.Fatal("after recovery the mail is not filed or the backfill not finished")
	}
}

// The same outage in the silent-close phase.
func TestAnOutageInTheClosePhaseIsRetriedNotSkipped(t *testing.T) {
	b := setupBackfill(t)
	missed := b.writes(t, 40)
	b.seat(t, b.carol, b.silent)
	b.run(t)
	b.WsExec(t, `UPDATE deal_correction SET applied_at = $2 WHERE deal_id = $1`, b.silent, b.now.AddDate(0, 0, -10))
	b.setBackfillFinished(t, false)
	brain := &flakyTopic{down: true}

	if !b.passFails(t, brain) {
		t.Fatal("the pass did not fail while the model was down")
	}
	if b.filedOn(t, missed, b.silent) || b.finished(t) {
		t.Fatal("an outage filed the mail or finished the backfill")
	}
	brain.down = false
	if b.passFails(t, brain) {
		t.Fatal("the pass failed after the model came back")
	}
	if !b.filedOn(t, missed, b.silent) || !b.finished(t) {
		t.Fatal("after recovery the mail is not filed or the backfill not finished")
	}
	if n := b.WsCount(t, `SELECT count(*) FROM deal_correction WHERE deal_id = $1 AND reoffered_at IS NOT NULL`, b.silent); n != 1 {
		t.Fatalf("the close was offered back %d times, want once", n)
	}
}

// Mail before a close goes on the deal that was closed and on no deal opened
// since, even when the same stakeholder sits on both.
func TestTheClosePhaseFilesOnTheClosedDealAndNoOtherDeal(t *testing.T) {
	b := setupBackfill(t)
	missed := b.writes(t, 40)
	b.seat(t, b.carol, b.silent)
	b.run(t)
	b.WsExec(t, `UPDATE deal_correction SET applied_at = $2 WHERE deal_id = $1`, b.silent, b.now.AddDate(0, 0, -10))
	pipeline := ids.From[ids.PipelineKind](ids.MustParse(b.WsScalar(t, `SELECT pipeline_id::text FROM deal WHERE id = $1`, b.supported)))
	open := ids.From[ids.StageKind](ids.MustParse(b.WsScalar(t, `SELECT stage_id::text FROM deal WHERE id = $1`, b.supported)))
	created, err := b.Deals.CreateDeal(b.Admin(), deals.CreateDealInput{Name: "Opened after the close", Source: "manual", PipelineID: pipeline, StageID: open})
	if err != nil {
		t.Fatal(err)
	}
	later := ids.UUID(created.Id)
	b.seat(t, b.carol, later)
	b.setBackfillFinished(t, false)

	b.pass(t, true)
	if !b.filedOn(t, missed, b.silent) {
		t.Fatal("the mail is not on the deal that was closed")
	}
	if b.filedOn(t, missed, later) {
		t.Fatal("mail from before the close was filed on a deal opened after it")
	}
}

// The check must confirm the closed deal. A model that says the mail is about
// none of the offered deals files nothing and offers nothing.
func TestTheClosePhaseFilesNothingTheTopicCheckDoesNotConfirm(t *testing.T) {
	b := setupBackfill(t)
	missed := b.writes(t, 40)
	b.seat(t, b.carol, b.silent)
	b.run(t)
	b.WsExec(t, `UPDATE deal_correction SET applied_at = $2 WHERE deal_id = $1`, b.silent, b.now.AddDate(0, 0, -10))
	b.setBackfillFinished(t, false)

	b.passPaged(t, true, capture.DealRefileLimit, notAboutAnyDeal)
	if b.filedOn(t, missed, b.silent) {
		t.Fatal("mail the check did not confirm was filed on the closed deal")
	}
	if n := b.WsCount(t, `SELECT count(*) FROM deal_correction WHERE deal_id = $1 AND reoffered_at IS NOT NULL`, b.silent); n != 0 {
		t.Fatal("a close was offered back without any mail filed")
	}
}

// A thread only its participants may read is filed too, and the close is
// offered back from its date alone.
func TestTheClosePhaseFilesAParticipantsOnlyThread(t *testing.T) {
	b := setupBackfill(t)
	b.WsExec(t, `UPDATE capture_connection SET mail_posture = 'held' WHERE user_id = $1`, b.o.owner)
	missed := b.writes(t, 40)
	b.seat(t, b.carol, b.silent)
	b.run(t)
	b.WsExec(t, `UPDATE deal_correction SET applied_at = $2 WHERE deal_id = $1`, b.silent, b.now.AddDate(0, 0, -10))
	b.setBackfillFinished(t, false)

	b.pass(t, true)
	if !b.filedOn(t, missed, b.silent) {
		t.Fatal("the participants-only thread is not filed on the closed deal")
	}
	if n := b.WsCount(t, `SELECT count(*) FROM deal_correction WHERE deal_id = $1 AND reoffered_at IS NOT NULL`, b.silent); n != 1 {
		t.Fatalf("the close was offered back %d times, want once", n)
	}
}

// The window is the one the pass froze at its start. A change to the setting
// between the phases moves neither the mail it files nor the offer.
func TestTheOfferUsesTheWindowThePassFroze(t *testing.T) {
	b := setupBackfill(t)
	missed := b.writes(t, 100)
	b.seat(t, b.carol, b.silent)
	b.run(t)
	b.WsExec(t, `UPDATE deal_correction SET applied_at = $2 WHERE deal_id = $1`, b.silent, b.now.AddDate(0, 0, -10))
	b.setBackfillFinished(t, false)
	worker := newCapturedDealRefileWorker(b.Pool, aboutEveryDeal)
	ctx := principal.WithCorrelationID(principal.SystemActing(b.Admin(), dealRefileActor), ids.NewV7())
	if _, err := worker.backfillState(ctx, CapturedDealRefileArgs{AsOf: b.now}); err != nil {
		t.Fatal(err)
	}
	err := database.WithTx(b.Admin(), b.Pool, func(tx pgx.Tx) error {
		return NewSettingsStore(b.Pool).SetRawTx(b.Admin(), tx, "deals.silent_close_days", []byte("60"))
	})
	if err != nil {
		t.Fatal(err)
	}

	b.pass(t, true)
	if !b.filedOn(t, missed, b.silent) {
		t.Fatal("the frozen 120-day window did not file mail from 100 days before the close")
	}
	if n := b.WsCount(t, `SELECT count(*) FROM deal_correction WHERE deal_id = $1 AND reoffered_at IS NOT NULL`, b.silent); n != 1 {
		t.Fatalf("the close was offered back %d times, want once under the frozen window", n)
	}
}

// The open phase reads mail from the window's two edges: nothing older than it
// and nothing after the pass's instant.
func TestTheOpenPhaseReadsBothEdgesOfItsWindow(t *testing.T) {
	b := setupBackfill(t)
	inside := b.writes(t, 60)
	tooOld := b.writes(t, 130)
	ahead := b.writes(t, -5)
	b.seat(t, b.carol, b.silent)
	b.setBackfillFinished(t, false)

	var listed []ids.UUID
	err := database.WithTx(b.Admin(), b.Pool, func(tx pgx.Tx) error {
		var err error
		listed, err = mailDealRepair(b.Pool).StakeholderBackfillCandidatesTx(
			principal.SystemActing(b.Admin(), dealRefileActor), tx, b.now, 120, ids.Nil, ids.MustParse("ffffffff-ffff-ffff-ffff-ffffffffffff"), 100)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	if slices.Contains(listed, ahead) || slices.Contains(listed, tooOld) || !slices.Contains(listed, inside) {
		t.Errorf("the open phase lists %v, want only the mail inside both edges (%v)", listed, inside)
	}
	b.pass(t, true)
	if !b.filedOn(t, inside, b.silent) {
		t.Error("mail inside the window is not filed")
	}
	if b.filedOn(t, tooOld, b.silent) {
		t.Error("mail older than the window was filed")
	}
}

// notAboutAnyDeal is a topic model sure the mail fits none of the deals asked.
var notAboutAnyDeal = dealTopicStub{answer: func(ids []string) string {
	verdicts := make([]string, len(ids))
	for i := range verdicts {
		verdicts[i] = dealTopicNotAbout
	}
	return verdictsReply(0.95, verdicts...)(ids)
}}

// A close offered back lists at the instant it came back, so a page of newer
// receipts does not push it off Home. The receipt still dates the close itself.
func TestAnOfferedBackCloseOutranksNewerReceiptsOnHome(t *testing.T) {
	b := setupBackfill(t)
	b.writes(t, 40)
	b.seat(t, b.carol, b.silent)
	b.refuses(t, b.supported, b.now.AddDate(0, 0, -2))
	if got := b.run(t); got != 2 {
		t.Fatalf("setup: closed %d deals, want the silent one and the refused one", got)
	}
	closedAt := b.now.AddDate(0, 0, -10)
	b.WsExec(t, `UPDATE deal_correction SET applied_at = $2 WHERE deal_id = $1`, b.silent, closedAt)
	b.setBackfillFinished(t, false)
	b.pass(t, true)

	receipts := attentionReceipts{svc: approvals.NewService(InstallationDB(b.Pool)), deals: b.Deals}
	first, err := receipts.Recent(b.owner(t), b.now.AddDate(0, 0, -365), 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(first) != 1 || first[0].TargetID != b.silent {
		t.Fatalf("the first Home receipt is %+v, want the offered-back close of the silent deal", first)
	}
	if !first[0].OccurredAt.Equal(closedAt) {
		t.Errorf("the receipt dates the close at %v, want the close instant %v", first[0].OccurredAt, closedAt)
	}
}

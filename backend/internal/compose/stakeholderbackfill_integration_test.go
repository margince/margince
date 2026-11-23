// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// The stakeholder mail backfill: mail of a deal's stakeholder that capture
// kept off the deal, and the silent close that missing mail caused. Mail
// arrives through the production sink and the pass runs as the refile job.

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
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
	err := database.WithWorkspaceTx(c.Admin(), c.Pool, func(tx pgx.Tx) error {
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
func (b *backfillEnv) writes(t *testing.T, daysAgo int, from string) ids.UUID {
	t.Helper()
	return b.o.capture(t, owedMail{
		from: from, to: b.o.seat, subject: "Rollout plan", at: b.now.AddDate(0, 0, -daysAgo),
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
	err := database.WithWorkspaceTx(b.Admin(), b.Pool, func(tx pgx.Tx) error {
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
	within := b.writes(t, 60, stakeholderAddress)
	beyond := b.writes(t, 150, stakeholderAddress)
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
	missed := b.writes(t, 40, stakeholderAddress)
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
	first := b.writes(t, 60, stakeholderAddress)
	second := b.writes(t, 70, stakeholderAddress)
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

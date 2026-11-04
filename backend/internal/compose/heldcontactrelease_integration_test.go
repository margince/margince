// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// When a contact a confidentiality hold kept to its owner goes to the
// workspace. Each test opens a confidentiality question on a thread and judges
// its sender. Then the confidentiality engine clears or keeps the thread.

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/datatask"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// heldExchange is one message on a thread whose confidentiality question is
// still open, and that question.
type heldExchange struct {
	activity, question ids.UUID
}

func seedHeldExchange(t *testing.T, e *integration.Env, email, thread string) heldExchange {
	t.Helper()
	activity := seedHeldThreadMail(t, e, thread, email, "Rahmenvertrag")
	return heldExchange{activity: activity, question: seedThreadQuestion(t, e, thread, activity)}
}

func heldContactID(t *testing.T, e *integration.Env, email string) ids.UUID {
	t.Helper()
	var id ids.UUID
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(), `
			SELECT p.id FROM contact p JOIN contact_email pe ON pe.contact_id = p.id
			 WHERE pe.email = $1 AND p.archived_at IS NULL`, email).Scan(&id)
	}); err != nil {
		t.Fatalf("reading the contact of %s: %v", email, err)
	}
	return id
}

func withheldRows(t *testing.T, e *integration.Env, email string) int {
	t.Helper()
	return countIn(t, e, `SELECT count(*) FROM capture_pending_counterparty
		WHERE email = $1 AND withheld_from_workspace`, email)
}

func requirePublished(t *testing.T, e *integration.Env, email string) {
	t.Helper()
	if visibility, _ := contactVisibility(t, e, email); visibility != "workspace" {
		t.Fatalf("the contact of %s is %q, want workspace", email, visibility)
	}
	if reason := narrowingReasonOf(t, e, email); reason != nil {
		t.Errorf("a published contact still records narrowing reason %q", *reason)
	}
	if n := withheldRows(t, e, email); n != 0 {
		t.Errorf("%d ledger rows still withhold a sender whose contact was published", n)
	}
}

func reconcileHeldContacts(t *testing.T, e *integration.Env) {
	t.Helper()
	engine := NewConfidentialityVerdictEngine(e.Pool, nil, slog.Default())
	if _, err := engine.FinishSettledThreads(principal.WithWorkspaceID(context.Background(), e.WS)); err != nil {
		t.Fatalf("the confidentiality reconciling pass: %v", err)
	}
}

func TestAClearedThreadPublishesTheContactItsHoldKept(t *testing.T) {
	e := setupWithOwnMailbox(t)
	const email = "einkauf@heldfirst.example"
	held := seedHeldExchange(t, e, email, "thr-held-first")
	judge(t, e, email, capture.KindContact, held.activity)
	requireNarrowed(t, e, email, contacts.NarrowedConfidentialityHold)
	before := heldContactID(t, e, email)

	runConfidentiality(t, e, held.question, confidentialityOrdinary, 0.95)

	requirePublished(t, e, email)
	if after := heldContactID(t, e, email); after != before {
		t.Errorf("the release made contact %s, want the held contact %s", after, before)
	}
	if n := countIn(t, e, `SELECT count(*) FROM audit_log
		WHERE entity_type = 'contact' AND entity_id = $1 AND after ->> 'visibility' = 'workspace'`, before); n != 1 {
		t.Errorf("%d audit rows record the publication, want 1", n)
	}
	if n := countIn(t, e, `SELECT count(*) FROM activity_link
		WHERE entity_type = 'contact' AND contact_id = $1 AND activity_id = $2`, before, held.activity); n != 1 {
		t.Errorf("the published contact is linked to %d copies of its message, want 1", n)
	}
}

// The thread clears before the sender is judged: the verdict publishes at
// once, and the end state is the one the other order reaches.
func TestAThreadClearedBeforeItsSenderIsJudgedEndsTheSame(t *testing.T) {
	e := setupWithOwnMailbox(t)
	const email = "einkauf@clearedfirst.example"
	held := seedHeldExchange(t, e, email, "thr-cleared-first")
	runConfidentiality(t, e, held.question, confidentialityOrdinary, 0.95)

	judge(t, e, email, capture.KindContact, held.activity)

	requirePublished(t, e, email)
}

// Mail the owner sent to an address that never answered is kept the owner's
// for that reason once the hold ends.
func TestAClearedThreadKeepsTheNoAnswerRuleForMailTheOwnerSent(t *testing.T) {
	e := setupWithOwnMailbox(t)
	const email = "prospect@heldoutbound.example"
	held := seedHeldExchange(t, e, email, "thr-held-outbound")
	e.WsExec(t, `UPDATE activity SET direction = 'outbound', counterparty_outbound_attested = true
		WHERE id = $1`, held.activity)
	judge(t, e, email, capture.KindContact, held.activity)
	requireNarrowed(t, e, email, contacts.NarrowedConfidentialityHold)

	runConfidentiality(t, e, held.question, confidentialityOrdinary, 0.95)

	requireNarrowed(t, e, email, contacts.NarrowedOutboundNoAnswer)
	if n := withheldRows(t, e, email); n == 0 {
		t.Error("the ledger stopped withholding a sender whose contact is still the owner's")
	}
}

// A second answer about the same sender came from another held thread. One
// thread clearing does not end the hold the other still makes.
func TestAContactStaysHeldWhileAnotherThreadHoldsIt(t *testing.T) {
	e := setupWithOwnMailbox(t)
	const email = "partner@twothreads.example"
	first := seedHeldExchange(t, e, email, "thr-two-a")
	judge(t, e, email, capture.KindContact, first.activity)
	second := seedHeldThreadMail(t, e, "thr-two-b", email, "Vertraulich")
	seedThreadHold(t, e, "thr-two-b", "held")
	judge(t, e, email, capture.KindContact, second)

	runConfidentiality(t, e, first.question, confidentialityOrdinary, 0.95)
	reconcileHeldContacts(t, e)

	requireNarrowed(t, e, email, contacts.NarrowedConfidentialityHold)
}

// An owner who makes a released contact private again decided it. A later
// release over the same sender is no answer to that.
func TestTheOwnersOwnPrivateChoiceSurvivesALaterRelease(t *testing.T) {
	e := setupWithOwnMailbox(t)
	const email = "berater@ownerchoice.example"
	held := seedHeldExchange(t, e, email, "thr-owner-choice")
	judge(t, e, email, capture.KindContact, held.activity)
	runConfidentiality(t, e, held.question, confidentialityOrdinary, 0.95)
	requirePublished(t, e, email)
	id := ids.From[ids.ContactKind](heldContactID(t, e, email))
	if _, err := e.Contacts.UpdateContact(purgeCtx(e, e.Rep1), id, contacts.UpdateContactInput{Visibility: new("owner")}); err != nil {
		t.Fatalf("making the contact private by hand: %v", err)
	}
	e.WsExec(t, `UPDATE capture_pending_counterparty SET withheld_from_workspace = true WHERE email = $1`, email)

	reconcileHeldContacts(t, e)

	requireNarrowed(t, e, email, contacts.NarrowedHumanDecided)
}

// A thread judged sensitive keeps holding, and so does its contact.
func TestASensitiveThreadKeepsItsContactHeld(t *testing.T) {
	e := setupWithOwnMailbox(t)
	const email = "anwalt@sensitive.example"
	held := seedHeldExchange(t, e, email, "thr-sensitive")
	judge(t, e, email, capture.KindContact, held.activity)

	runConfidentiality(t, e, held.question, confidentialityLegal, 0.95)
	reconcileHeldContacts(t, e)

	requireNarrowed(t, e, email, contacts.NarrowedConfidentialityHold)
}

// clearedWithoutRelease clears a held contact's thread through the engine. Then
// it puts back the state from before the release existed: the contact held and
// its sender withheld. No writer produces that state any more.
func clearedWithoutRelease(t *testing.T, e *integration.Env, email string, held heldExchange) {
	t.Helper()
	runConfidentiality(t, e, held.question, confidentialityOrdinary, 0.95)
	e.WsExec(t, `UPDATE contact SET visibility = 'owner', narrowing_reason = 'confidentiality_hold'
		WHERE id = $1`, heldContactID(t, e, email))
	e.WsExec(t, `UPDATE capture_pending_counterparty SET withheld_from_workspace = true
		WHERE email = $1 AND status = 'real'`, email)
}

func TestTheReconcilingPassReleasesAHoldNoReleaseReached(t *testing.T) {
	e := setupWithOwnMailbox(t)
	const email = "einkauf@missedrelease.example"
	held := seedHeldExchange(t, e, email, "thr-missed")
	judge(t, e, email, capture.KindContact, held.activity)
	clearedWithoutRelease(t, e, email, held)

	reconcileHeldContacts(t, e)

	requirePublished(t, e, email)
}

func TestTheHeldContactTaskReleasesAcrossPagesAndReplaysSafely(t *testing.T) {
	e := setupWithOwnMailbox(t)
	const email = "einkauf@historic.example"
	held := seedHeldExchange(t, e, email, "thr-historic")
	judge(t, e, email, capture.KindContact, held.activity)
	clearedWithoutRelease(t, e, email, held)
	const keptEmail = "anwalt@historic.example"
	kept := seedHeldExchange(t, e, keptEmail, "thr-historic-kept")
	judge(t, e, keptEmail, capture.KindContact, kept.activity)
	for i := range 7 {
		if _, err := e.Contacts.CreateContact(e.Admin(), contacts.CreateContactInput{
			FullName: fmt.Sprintf("Unrelated %d", i), Source: "manual",
		}); err != nil {
			t.Fatal(err)
		}
	}
	task := heldContactsDataTask(e.Pool)
	task.Steps[0].Limit, task.Steps[0].MinLimit = 2, 1
	ctx := datatask.Acting(e.Admin(), task.ID)

	runMachineAddressTask(ctx, t, e, task)

	requirePublished(t, e, email)
	requireNarrowed(t, e, keptEmail, contacts.NarrowedConfidentialityHold)
	if n := e.WsCount(t, `SELECT count(*) FROM data_task_run
		WHERE task_id = $1 AND status = 'done' AND scanned > 2`, task.ID); n != 1 {
		t.Errorf("the task finished %d times having read more than one page, want once", n)
	}
	audits := e.WsCount(t, `SELECT count(*) FROM audit_log`)
	if err := database.WithWorkspaceTx(ctx, e.Pool, func(tx pgx.Tx) error {
		out, err := task.Steps[0].Batch(ctx, tx, datatask.BatchIn{Limit: 100, RunID: ids.NewV7()})
		if err == nil && out.Changed != 0 {
			t.Errorf("replay changed %d contacts", out.Changed)
		}
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if again := e.WsCount(t, `SELECT count(*) FROM audit_log`); again != audits {
		t.Fatal("replay wrote more audit rows")
	}
}

// The owner shares a thread the classifier has not answered yet. The hold the
// open question made ends with the share.
func TestAnOwnersShareReleasesTheContactTheThreadHeld(t *testing.T) {
	e := setupWithOwnMailbox(t)
	const email = "einkauf@ownershare.example"
	held := seedHeldExchange(t, e, email, "thr-owner-share")
	judge(t, e, email, capture.KindContact, held.activity)
	requireNarrowed(t, e, email, contacts.NarrowedConfidentialityHold)

	outcome, err := NewThreadAudienceSetter(e.Pool).Decide(purgeCtx(e, e.Rep1), "thr-owner-share", true)
	if err != nil || !outcome.Shared {
		t.Fatalf("sharing the thread: %+v, %v", outcome, err)
	}

	requirePublished(t, e, email)
}

// A confidential thread that arrives after the address was judged gets no
// ledger row of its own. It still holds the contact when the first thread clears.
func TestConfidentialMailAfterASettledVerdictKeepsTheContactHeld(t *testing.T) {
	e := setupWithOwnMailbox(t)
	const email = "einkauf@latehold.example"
	first := seedHeldExchange(t, e, email, "thr-late-a")
	judge(t, e, email, capture.KindContact, first.activity)
	late := seedHeldThreadMail(t, e, "thr-late-b", email, "Vertraulich")
	seedThreadQuestion(t, e, "thr-late-b", late)

	runConfidentiality(t, e, first.question, confidentialityOrdinary, 0.95)
	reconcileHeldContacts(t, e)

	requireNarrowed(t, e, email, contacts.NarrowedConfidentialityHold)
}

// seedSettledDisposition plants a settled ledger row for an address, as a
// verdict for that address leaves one.
func seedSettledDisposition(t *testing.T, e *integration.Env, email, status string, kind *string, activity ids.UUID) {
	t.Helper()
	e.WsExec(t, `INSERT INTO capture_pending_counterparty (email, domain, activity_id, owner_id, status, kind, resolved_at)
		VALUES ($1, 'second.example', $2, $3, $4, $5, now())`, email, activity, e.Rep1, status, kind)
}

// A contact with a second address is released only when that address is a
// proven business contact too. Any other answer about it keeps the hold.
func TestASecondAddressWithAnyOtherAnswerKeepsTheContactHeld(t *testing.T) {
	kind := func(k string) *string { return &k }
	cases := []struct {
		name, status string
		kind         *string
		published    bool
	}{
		{"contact", "real", kind(capture.KindContact), true},
		{"personal", "real", kind("personal"), false},
		{"advisor", "real", kind(capture.KindAdvisor), false},
		{"rejected", "rejected", kind(capture.KindContact), false},
		{"unknown kind", "real", nil, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			e := setupWithOwnMailbox(t)
			const email, second = "einkauf@twoaddresses.example", "privat@second.example"
			held := seedHeldExchange(t, e, email, "thr-two-addr")
			judge(t, e, email, capture.KindContact, held.activity)
			id := ids.From[ids.ContactKind](heldContactID(t, e, email))
			if _, err := e.Contacts.UpdateContact(purgeCtx(e, e.Rep1), id, contacts.UpdateContactInput{Emails: []contacts.ContactEmailInput{
				{Email: email, EmailType: "work", IsPrimary: true}, {Email: second, EmailType: "work", Position: 1},
			}}); err != nil {
				t.Fatalf("adding the second address: %v", err)
			}
			seedSettledDisposition(t, e, second, c.status, c.kind, seedThreadedMail(t, e, second, "Hallo", "inbound", "thr-second"))

			runConfidentiality(t, e, held.question, confidentialityOrdinary, 0.95)
			reconcileHeldContacts(t, e)

			if visibility, _ := contactVisibility(t, e, email); (visibility == "workspace") != c.published {
				t.Errorf("with a %s answer on the second address the contact is %q", c.name, visibility)
			}
		})
	}
}

// releaseUnder runs one release with a short lock wait, as the reconciling pass
// would, and returns its error.
func releaseUnder(e *integration.Env, email string, id ids.ContactID) error {
	ctx := principal.SystemActing(principal.WithWorkspaceID(context.Background(), e.WS), confidentialityActor)
	return database.WithWorkspaceTx(ctx, e.Pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `SET LOCAL lock_timeout = '300ms'`); err != nil {
			return err
		}
		_, err := releaseHeldContactTx(ctx, tx, newCounterpartyStore(e.Pool), id)
		return err
	})
}

func isLockTimeout(err error) bool {
	pgErr, ok := errors.AsType[*pgconn.PgError](err)
	return ok && pgErr.Code == "55P03"
}

// An owner holding the thread again while a release runs: the release waits
// for the owner, then reads the hold.
func TestAReleaseWaitsForAnOwnerHoldingTheThreadAgain(t *testing.T) {
	e := setupWithOwnMailbox(t)
	const email = "einkauf@rehold.example"
	held := seedHeldExchange(t, e, email, "thr-rehold")
	judge(t, e, email, capture.KindContact, held.activity)
	clearedWithoutRelease(t, e, email, held)
	id := ids.From[ids.ContactKind](heldContactID(t, e, email))
	owner := purgeCtx(e, e.Rep1)

	err := database.WithWorkspaceTx(owner, e.Pool, func(tx pgx.Tx) error {
		if err := capture.NewThreadVerdictStore(InstallationDB(e.Pool)).DecideAsOwner(owner, tx, "thr-rehold", false); err != nil {
			return err
		}
		if err := releaseUnder(e, email, id); !isLockTimeout(err) {
			t.Errorf("a release during the owner's re-hold returned %v, want a lock wait", err)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("holding the thread again: %v", err)
	}
	reconcileHeldContacts(t, e)

	requireNarrowed(t, e, email, contacts.NarrowedConfidentialityHold)
}

// New mail from the address while a release runs waits for the release. So it
// never falls between what the release read and what it wrote.
func TestNewMailFromTheAddressWaitsForARunningRelease(t *testing.T) {
	e := setupWithOwnMailbox(t)
	const email = "einkauf@newmail.example"
	held := seedHeldExchange(t, e, email, "thr-newmail")
	judge(t, e, email, capture.KindContact, held.activity)
	clearedWithoutRelease(t, e, email, held)
	id := ids.From[ids.ContactKind](heldContactID(t, e, email))
	ctx := principal.SystemActing(principal.WithWorkspaceID(context.Background(), e.WS), confidentialityActor)

	err := database.WithWorkspaceTx(ctx, e.Pool, func(tx pgx.Tx) error {
		if _, err := releaseHeldContactTx(ctx, tx, newCounterpartyStore(e.Pool), id); err != nil {
			return err
		}
		waiting, cancel := context.WithTimeout(mailboxOwnerCtx(e, e.Rep1), 500*time.Millisecond)
		defer cancel()
		_, err := newCaptureSink(e.Pool, CaptureConfig{}).Upsert(waiting, connector.NormalizedRecord{
			EntityType:   "activity",
			NaturalKey:   connector.NaturalKey{SourceSystem: connector.EmailSourceSystem, SourceID: "newmail-during-release"},
			Counterparty: connector.Counterparty{Email: email, Domain: "newmail.example", Direction: connector.DirectionInbound},
			Fields:       capture.ActivityFields{Kind: "email", Subject: "Vertraulich", Body: "Anbei.", Direction: connector.DirectionInbound},
			ThreadKey:    "thr-newmail-b",
			Addresses:    []string{email, "a@authz.test"},
			Source:       "gmail:newmail-during-release", CapturedBy: "connector:gmail",
		})
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Errorf("a capture during a release returned %v, want it to wait for the release", err)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("running the release: %v", err)
	}
}

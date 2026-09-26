// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// The quiet-record scan claims silence only where Margince can see: a record
// whose owner has no live mailbox, or whose mailbox is still importing its
// history, is not drawn, because the silence on its timeline may be nothing
// more than mail Margince never received.

import (
	"context"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// connectMailbox gives one seat a capture connection on the provider, in the
// given status, with one history import in the given status ("" for none).
// Idempotent per (seat, provider), so a scan helper can call it every pass.
func connectMailbox(t *testing.T, e *Env, user ids.UUID, provider, status, backfill string) {
	t.Helper()
	e.WsExec(t, `
		WITH conn AS (
		  INSERT INTO capture_connection (user_id, provider, status, credential_ref)
		  VALUES ($1, $2, $3, 'vault:test')
		  ON CONFLICT DO NOTHING
		  RETURNING id)
		INSERT INTO capture_backfill (connection_id, window_months, after_date, status)
		SELECT conn.id, 12, DATE '2025-07-01', $4 FROM conn WHERE $4 <> ''`,
		user, provider, status, backfill)
}

// connectCaughtUpMailbox is the ordinary state of a seat that has used
// Margince for a while: gmail connected, its history import finished.
func connectCaughtUpMailbox(t *testing.T, e *Env, user ids.UUID) {
	t.Helper()
	connectMailbox(t, e, user, "gmail", "connected", "done")
}

// seedQuietOwnedDeal is one open deal owned by the seat, quiet since
// quietSince and established long before the cutoff.
func seedQuietOwnedDeal(t *testing.T, e *Env, owner *ids.UUID) ids.UUID {
	t.Helper()
	conn := OwnerConn(t)
	pipeline, open, _ := DealFixture(t, e)
	deal := e.SeedDeal(t, "Quiet Owned Deal", pipeline, open, owner)
	backdateCreatedAt(t, conn, "deal", deal, longEstablished)
	linkQuietTouch(t, conn, e.WS, "deal", deal)
	seedNoActivityReminder(t, conn, e.WS)
	return deal
}

func TestAQuietDealIsDrawnOnlyWhileItsOwnersMailIsVisible(t *testing.T) {
	cases := []struct {
		name     string
		provider string
		status   string
		backfill string
		want     int
		why      string
	}{
		{"no connection", "", "", "", 0, "the owner never connected a mailbox, so the silence is unproven"},
		{"history imported", "gmail", "connected", "done", 1, "a caught-up mailbox makes the silence real"},
		{"no import ever run", "imap", "connected", "", 1, "a live mailbox with no import pending sees new mail"},
		{"history still importing", "gmail", "connected", "running", 0, "the import has not reached the mail that may answer it"},
		{"history import queued", "graph", "connected", "queued", 0, "the import has not started"},
		{"connection failing", "gmail", "error", "done", 0, "a failing connection delivers nothing"},
		{"calendar only", "gcal", "connected", "done", 0, "a calendar carries no mail"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			e := Setup(t)
			seat := e.Rep1
			deal := seedQuietOwnedDeal(t, e, &seat)
			if tc.provider != "" {
				connectMailbox(t, e, seat, tc.provider, tc.status, tc.backfill)
			}

			runEligibilityScan(t, e)

			if got := taskCountOn(t, e, "deal", deal); got != tc.want {
				t.Fatalf("reminder tasks = %d, want %d — %s", got, tc.want, tc.why)
			}
			if tc.want == 0 {
				return
			}
			// The reminder says which silence it is about, so its owner can
			// judge it against their own sent mail.
			want := "Check in — no activity since " + quietSince.Format(time.DateOnly)
			if got := reminderSubjectOn(t, e, deal); got != want {
				t.Errorf("reminder subject = %q, want %q", got, want)
			}
		})
	}
}

// One finished mailbox does not vouch for a second one still importing: the
// importing one is missing exactly the mail that could answer the reminder.
func TestAnyMailboxStillImportingHoldsTheOwnersRecordsBack(t *testing.T) {
	e := Setup(t)
	seat := e.Rep1
	deal := seedQuietOwnedDeal(t, e, &seat)
	connectMailbox(t, e, seat, "gmail", "connected", "done")
	connectMailbox(t, e, seat, "graph", "connected", "running")

	runEligibilityScan(t, e)

	if got := taskCountOn(t, e, "deal", deal); got != 0 {
		t.Fatalf("reminder tasks = %d, want 0 — the graph mailbox is still importing", got)
	}
}

// An unowned record keeps the old behaviour: its reminder goes to the
// unassigned queue, and no one seat's mailbox could prove it wrong.
func TestAQuietUnownedDealIsStillDrawn(t *testing.T) {
	e := Setup(t)
	deal := seedQuietOwnedDeal(t, e, nil)
	e.WsExec(t, `UPDATE deal SET owner_id = NULL WHERE id = $1`, deal)

	runEligibilityScan(t, e)

	if got := taskCountOn(t, e, "deal", deal); got != 1 {
		t.Fatalf("reminder tasks on an unowned quiet deal = %d, want 1", got)
	}
}

// An account only absorbs its deals while it is itself drawn. An account held
// back because its owner's mail is not visible must not take its deal's
// reminder down with it when the deal's owner can be seen.
func TestAnAccountHeldBackForItsOwnersMailboxDoesNotAbsorbItsDeal(t *testing.T) {
	e := Setup(t)
	conn := OwnerConn(t)
	dealOwner, accountOwner := e.Rep1, e.Rep2
	deal := seedQuietOwnedDeal(t, e, &dealOwner)
	company := e.SeedCompany(t, "Unseen Account", &accountOwner)
	attachDealToCompany(t, conn, deal, company)
	backdateCreatedAt(t, conn, "company", company, longEstablished)
	linkQuietTouch(t, conn, e.WS, "company", company)
	connectCaughtUpMailbox(t, e, dealOwner)

	runEligibilityScan(t, e)

	if got := taskCountOn(t, e, "company", company); got != 0 {
		t.Errorf("reminder tasks on the account = %d, want 0 — its owner's mail is not visible", got)
	}
	if got := taskCountOn(t, e, "deal", deal); got != 1 {
		t.Errorf("reminder tasks on the deal = %d, want 1 — the account is not drawn, so it cannot absorb the deal", got)
	}
}

// reminderSubjectOn reads the subject of the one task on the deal's timeline.
func reminderSubjectOn(t *testing.T, e *Env, deal ids.UUID) string {
	t.Helper()
	var subject string
	if err := OwnerConn(t).QueryRow(context.Background(), `
		SELECT a.subject FROM activity a
		JOIN activity_link al ON al.activity_id = a.id
		WHERE al.deal_id = $1 AND a.kind = 'task'`, deal).Scan(&subject); err != nil {
		t.Fatalf("reading the reminder subject: %v", err)
	}
	return subject
}

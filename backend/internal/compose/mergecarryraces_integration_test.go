// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// A write that names the retired record while its merge is in flight lands on
// the survivor or is refused. It never lands on the retired record after the
// carry has already passed it.
//
// Each test parks a REAL merge inside its transaction, after its consent carry
// and before it commits, by holding a row a later step of the merge must lock.
// The writer then starts against a record that still reads as live, waits on
// the merge, and resumes after it commits. That is the window the carry cannot
// see: a writer that did not hold its subject would insert onto the retired
// record there and strand the row.

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/consent"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// parkedMerge is a merge waiting on a row the test holds.
type parkedMerge struct {
	holder   pgx.Tx
	mergePID int
	done     chan error
}

// parkContactMerge starts merging retired into survivor and parks it at its
// introduction carry, which locks every ask naming either contact: the test
// holds one naming the survivor.
func (c *carryEnv) parkContactMerge(t *testing.T, retired, survivor ids.ContactID) *parkedMerge {
	t.Helper()
	ask := c.ask(t, survivor, c.e.Rep1, nil)
	return c.parkMerge(t, `SELECT 1 FROM intro_request WHERE id = $1 FOR UPDATE`, ask, func() error {
		_, err := c.contacts.MergeContact(c.admin, retired, survivor)
		return err
	})
}

// parkLeadMerge parks a lead merge at the step that retires the loser's open
// review pairs; the test holds the pair the two leads already form.
func (c *carryEnv) parkLeadMerge(t *testing.T, loser, winner ids.LeadID) *parkedMerge {
	t.Helper()
	var pair ids.UUID
	if err := c.e.Pool.QueryRow(context.Background(), `
		SELECT id FROM dedupe_candidate WHERE left_lead_id = $1 OR right_lead_id = $1`, loser).Scan(&pair); err != nil {
		t.Fatalf("the two leads formed no review pair to park the merge on: %v", err)
	}
	return c.parkMerge(t, `SELECT 1 FROM dedupe_candidate WHERE id = $1 FOR UPDATE`, pair, func() error {
		_, err := c.contacts.MergeLead(c.admin, loser, winner)
		return err
	})
}

func (c *carryEnv) parkMerge(t *testing.T, hold string, row ids.UUID, merge func() error) *parkedMerge {
	t.Helper()
	ctx := context.Background()
	holder, err := c.e.Pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := holder.Rollback(context.Background()); err != nil && !errors.Is(err, pgx.ErrTxClosed) {
			t.Errorf("releasing the parked merge: %v", err)
		}
	})
	var holderPID int
	if err := holder.QueryRow(ctx, `SELECT pg_backend_pid()`).Scan(&holderPID); err != nil {
		t.Fatal(err)
	}
	if _, err := holder.Exec(ctx, hold, row); err != nil {
		t.Fatalf("holding the row the merge will wait on: %v", err)
	}
	done := make(chan error, 1)
	go func() { done <- merge() }()
	return &parkedMerge{holder: holder, mergePID: waitForBackendBlockedBy(t, holder, holderPID, done), done: done}
}

// waitsOnIt proves the writer is queued behind the parked merge, so the order
// the test describes is the order that ran.
func (p *parkedMerge) waitsOnIt(t *testing.T, writer <-chan error) {
	t.Helper()
	waitForBackendBlockedBy(t, p.holder, p.mergePID, writer)
}

// commit lets the merge finish and requires that it did.
func (p *parkedMerge) commit(t *testing.T) {
	t.Helper()
	if err := p.holder.Rollback(context.Background()); err != nil {
		t.Fatalf("releasing the parked merge: %v", err)
	}
	if err := within(t, p.done); err != nil {
		t.Fatalf("the parked merge failed: %v", err)
	}
}

func within(t *testing.T, done <-chan error) error {
	t.Helper()
	select {
	case err := <-done:
		return err
	case <-time.After(30 * time.Second):
		t.Fatal("nothing finished within 30s after the merge was released")
		return nil
	}
}

// waitForBackendBlockedBy returns a backend waiting on blocker.
func waitForBackendBlockedBy(t *testing.T, probe pgx.Tx, blocker int, done <-chan error) int {
	t.Helper()
	return waitForBackendsBlockedBy(t, probe, blocker, 1, done)
}

// waitForBackendsBlockedBy waits until at least want backends wait on blocker,
// and returns one of them. The statistics snapshot is cleared on every look,
// because a transaction keeps the first one it read.
func waitForBackendsBlockedBy(t *testing.T, probe pgx.Tx, blocker, want int, done <-chan error) int {
	t.Helper()
	pace := time.NewTicker(20 * time.Millisecond)
	defer pace.Stop()
	budget := time.After(30 * time.Second)
	for {
		ctx := context.Background()
		if _, err := probe.Exec(ctx, `SELECT pg_stat_clear_snapshot()`); err != nil {
			t.Fatalf("clearing the statistics snapshot: %v", err)
		}
		// Directly, or queued behind a backend that is: a second writer on the
		// same row waits on the first one's place in the queue.
		var waiting, waiter int
		if err := probe.QueryRow(ctx, `
			WITH direct AS (
				SELECT pid FROM pg_stat_activity
				 WHERE datname = current_database() AND $1 = ANY (pg_blocking_pids(pid)))
			SELECT count(*), coalesce(min(pid) FILTER (WHERE pid IN (SELECT pid FROM direct)), 0)
			  FROM pg_stat_activity
			 WHERE datname = current_database()
			   AND (pid IN (SELECT pid FROM direct) OR pg_blocking_pids(pid) && ARRAY(SELECT pid FROM direct))`,
			blocker).Scan(&waiting, &waiter); err != nil {
			t.Fatalf("reading who waits on backend %d: %v", blocker, err)
		}
		if waiting >= want {
			return waiter
		}
		select {
		case err := <-done:
			t.Fatalf("it finished (%v) without ever waiting on backend %d", err, blocker)
		case <-budget:
			t.Fatalf("%d of %d backend(s) waited on backend %d within 30s", waiting, want, blocker)
		case <-pace.C:
		}
	}
}

// pressStatus is pressUnsubscribe for a goroutine, which may not fail the test.
func (c *carryEnv) pressStatus(token string) error {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/public/preferences/"+token+"/unsubscribe",
		strings.NewReader("List-Unsubscribe=One-Click")).WithContext(c.publicCtx())
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	consent.NewHandlers(c.e.DB()).OneClickUnsubscribe(rec, req, token, crmcontracts.OneClickUnsubscribeParams{})
	if rec.Code != http.StatusOK {
		return errors.New(rec.Body.String())
	}
	return nil
}

// The press resolved its link to the retired contact before the merge
// committed, and withdraws once it has: it must withdraw the survivor.
func TestAnUnsubscribePressedDuringTheMergeWithdrawsTheSurvivor(t *testing.T) {
	c := setupCarry(t)
	retired := c.contactAt(t, "Race Unsub Retired", "race-unsub@carry.test")
	survivor := c.contactAt(t, "Race Unsub Survivor", "race-unsub-survivor@carry.test")
	newsletter := c.grantNewsletter(t, survivor)
	token := c.withdrawalLink(t, consent.WithdrawalMintInput{Address: "race-unsub@carry.test", ContactID: retired})

	merge := c.parkContactMerge(t, retired, survivor)
	pressed := make(chan error, 1)
	go func() { pressed <- c.pressStatus(token) }()
	merge.waitsOnIt(t, pressed)
	merge.commit(t)

	if err := within(t, pressed); err != nil {
		t.Fatalf("the press failed: %v", err)
	}
	if err := c.sendAllowed(newsletter, "race-unsub-survivor@carry.test"); !errors.Is(err, apperrors.ErrConsentNotGranted) {
		t.Fatalf("a newsletter to the survivor answers %v after the press, want it refused — "+
			"the withdrawal landed on the retired record", err)
	}
}

// A send resolving its recipient while the merge moves that address mints the
// preference link on the survivor.
func TestAPreferenceLinkMintedDuringTheMergeLandsOnTheSurvivor(t *testing.T) {
	c := setupCarry(t)
	retired := c.contactAt(t, "Race Pref Retired", "race-pref@carry.test")
	survivor := c.contactAt(t, "Race Pref Survivor", "race-pref-survivor@carry.test")

	merge := c.parkContactMerge(t, retired, survivor)
	minted := make(chan error, 1)
	var token string
	go func() {
		var err error
		token, _, err = c.consent.PreferenceTokenForEmail(c.admin, "race-pref@carry.test")
		minted <- err
	}()
	merge.waitsOnIt(t, minted)
	merge.commit(t)

	if err := within(t, minted); err != nil {
		t.Fatalf("the mint failed: %v", err)
	}
	var holder ids.UUID
	if err := c.e.Pool.QueryRow(context.Background(),
		`SELECT contact_id FROM preference_token WHERE token = $1`, token).Scan(&holder); err != nil {
		t.Fatal(err)
	}
	if holder != survivor.UUID {
		t.Fatalf("the link was minted on %s, want the survivor %s", holder, survivor)
	}
}

// An ask about the retired contact, made while the merge runs, is refused
// rather than written onto a record no read returns.
func TestAnAskMadeDuringTheMergeIsNotLeftOnTheRetiredContact(t *testing.T) {
	c := setupCarry(t)
	retired := c.contactAt(t, "Race Ask Retired", "race-ask@carry.test")
	survivor := c.contactAt(t, "Race Ask Survivor", "race-ask-survivor@carry.test")

	merge := c.parkContactMerge(t, retired, survivor)
	asked := make(chan error, 1)
	go func() {
		_, err := c.intros.Create(c.asker(), introductionsRequest(retired, c.e.Rep2))
		asked <- err
	}()
	merge.waitsOnIt(t, asked)
	merge.commit(t)

	if err := within(t, asked); !errors.Is(err, apperrors.ErrNotFound) {
		t.Fatalf("the ask answered %v, want not found for a contact merged away under it", err)
	}
	if n := c.count(t, `SELECT count(*) FROM intro_request WHERE contact_id = $1 OR through_contact_id = $1`, retired); n != 0 {
		t.Fatalf("%d ask(s) were left on the retired contact", n)
	}
}

// A hand-recorded exchange naming the retired contact is refused; a booking's
// inquiry follows the merge onto the survivor, because the booking happened.
func TestQualifyingEventsWrittenDuringTheMergeAreNotLeftBehind(t *testing.T) {
	c := setupCarry(t)
	retired := c.contactAt(t, "Race Event Retired", "race-event@carry.test")
	survivor := c.contactAt(t, "Race Event Survivor", "race-event-survivor@carry.test")
	booking := ids.NewV7()

	merge := c.parkContactMerge(t, retired, survivor)
	handWritten, inquired := make(chan error, 1), make(chan error, 1)
	go func() {
		_, err := c.consent.RecordQualifyingEvent(c.admin, retired, consent.RecordQualifyingEventInput{
			Kind: "in_person", Note: "met at the stand", OccurredAt: time.Now().Add(-time.Hour),
		})
		handWritten <- err
	}()
	merge.waitsOnIt(t, handWritten)
	go func() { inquired <- c.consent.RecordInquiry(c.admin, retired, booking) }()
	waitForBackendsBlockedBy(t, merge.holder, merge.mergePID, 2, inquired)
	merge.commit(t)

	if err := within(t, handWritten); !errors.Is(err, apperrors.ErrNotFound) {
		t.Errorf("the hand-recorded exchange answered %v, want not found", err)
	}
	if err := within(t, inquired); err != nil {
		t.Fatalf("the booking's inquiry failed: %v", err)
	}
	if n := c.count(t, `SELECT count(*) FROM consent_qualifying_event WHERE contact_id = $1`, retired); n != 0 {
		t.Errorf("%d qualifying event(s) were left on the retired contact", n)
	}
	if n := c.count(t, `SELECT count(*) FROM consent_qualifying_event WHERE contact_id = $1 AND source_entity_id = $2`,
		survivor, booking); n != 1 {
		t.Errorf("the survivor holds %d inquiry for the booking, want 1", n)
	}
}

// A lead's unsubscribe link minted while its lead merge runs is refused rather
// than stranded on the loser.
func TestALeadLinkMintedDuringTheLeadMergeIsNotLeftOnTheLoser(t *testing.T) {
	c := setupCarry(t)
	winner := c.lead(t, "Rita Race", "", "Race & Co")
	loser := c.lead(t, "Rita Racee", "rita@leadcarry.test", "Race & Co")

	merge := c.parkLeadMerge(t, loser, winner)
	minted := make(chan error, 1)
	go func() {
		minted <- c.e.DB().Tx(c.admin, func(tx pgx.Tx) error {
			_, err := c.consent.EnsureWithdrawalCredentialTx(c.admin, tx, consent.WithdrawalMintInput{
				Address: "rita@leadcarry.test", LeadID: loser, Scope: consent.WithdrawalScopeAllMarketing,
			})
			return err
		})
	}()
	merge.waitsOnIt(t, minted)
	merge.commit(t)

	if err := within(t, minted); !errors.Is(err, apperrors.ErrNotFound) {
		t.Fatalf("the mint answered %v, want not found for a lead merged away under it", err)
	}
	if n := c.count(t, `SELECT count(*) FROM withdrawal_credential WHERE lead_id = $1`, loser); n != 0 {
		t.Fatalf("%d link(s) were left on the merged-away lead", n)
	}
}

// An ask completed while the merge waits for it is no longer open when the
// merge decides, so the merge does not cancel it over the answer.
//
// The completion is spelled here as the one statement Complete commits,
// because the real writer cannot be paused between its write and its commit.
// That pause is the window: the merge must be waiting on the row, not reading
// a snapshot of it from before.
func TestAnAskCompletedWhileTheMergeWaitsIsNotCancelled(t *testing.T) {
	c := setupCarry(t)
	retired := c.contactAt(t, "Done Retired", "done@carry.test")
	survivor := c.contactAt(t, "Done Survivor", "done-survivor@carry.test")
	c.ask(t, survivor, c.e.Rep1, nil)
	completing := c.ask(t, retired, c.e.Rep1, nil)

	merge := c.parkMerge(t, `
		UPDATE intro_request SET status = 'introduced', introduced_at = now(),
		       version = version + 1, updated_at = now()
		 WHERE id = $1`, completing, func() error {
		_, err := c.contacts.MergeContact(c.admin, retired, survivor)
		return err
	})
	if err := merge.holder.Commit(context.Background()); err != nil {
		t.Fatalf("committing the completion: %v", err)
	}
	if err := within(t, merge.done); err != nil {
		t.Fatalf("the merge failed: %v", err)
	}

	if status, contact, _ := c.askStatus(t, completing); status != "introduced" || contact != survivor.UUID {
		t.Fatalf("the completed ask is %s on %s, want introduced and carried to the survivor", status, contact)
	}
}

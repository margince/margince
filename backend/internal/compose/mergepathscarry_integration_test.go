// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// Every merge-shaped path carries the consent links, and every one takes
// consent's stop lock before it locks a subject row.
//
// The contact merge was the only path that did either. A lead merge and a
// promotion left a lead's unsubscribe link and recorded bases on the retired
// lead, and both locked their lead row before reaching for the stop lock that
// recording a stop takes first — the inversion Postgres reports as a deadlock.

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/consent"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/commsauthz"
)

func (c *carryEnv) lead(t *testing.T, name, address, company string) ids.LeadID {
	t.Helper()
	in := contacts.CreateLeadInput{FullName: &name, CompanyName: &company, Source: "manual"}
	if address != "" {
		in.Email = &address
	}
	created, _, err := c.contacts.CreateLead(c.admin, in)
	if err != nil {
		t.Fatalf("creating lead %s: %v", name, err)
	}
	return ids.From[ids.LeadKind](ids.UUID(created.Id))
}

// leadHoldsLinkAndBasis gives a lead an unsubscribe link through the real
// writer, and the lawful-basis row recordBasis writes for a lead a send was
// authorized against.
func (c *carryEnv) leadHoldsLinkAndBasis(t *testing.T, lead ids.LeadID, address string) string {
	t.Helper()
	token := c.withdrawalLink(t, consent.WithdrawalMintInput{Address: address, LeadID: lead})
	if _, err := c.e.Pool.Exec(context.Background(), `
		INSERT INTO communication_basis (lead_id, kind, captured_by)
		VALUES ($1, 'subject_initiated_correspondence', 'human:test')`, lead); err != nil {
		t.Fatal(err)
	}
	return token
}

// subjectsHolding counts one subject arm's rows in the two tables a lead can hold.
func (c *carryEnv) subjectsHolding(t *testing.T, column string, id ids.UUID) (links, bases int) {
	t.Helper()
	// column is one of two compile-time literals chosen by the caller.
	links = c.count(t, `SELECT count(*) FROM withdrawal_credential WHERE `+column+` = $1`, id)
	bases = c.count(t, `SELECT count(*) FROM communication_basis WHERE `+column+` = $1`, id)
	return links, bases
}

func TestALeadMergeCarriesTheLeadsLinksAndBases(t *testing.T) {
	c := setupCarry(t)
	retired := c.lead(t, "Lena Merge", "lena@leadcarry.test", "Merge GmbH")
	survivor := c.lead(t, "Lena Survivor", "lena-survivor@leadcarry.test", "Merge GmbH")
	token := c.leadHoldsLinkAndBasis(t, retired, "lena@leadcarry.test")

	if _, err := c.contacts.MergeLead(c.admin, retired, survivor); err != nil {
		t.Fatalf("merging the leads: %v", err)
	}

	if links, bases := c.subjectsHolding(t, "lead_id", survivor.UUID); links != 1 || bases != 1 {
		t.Errorf("the surviving lead holds %d link(s) and %d basis row(s), want 1 and 1", links, bases)
	}
	ref, err := c.consent.ResolveWithdrawalToken(context.Background(), token)
	if err != nil || ref.LeadID != survivor {
		t.Errorf("the lead's unsubscribe link acts on %s (err=%v), want the surviving lead %s", ref.LeadID, err, survivor)
	}
}

// Promotion moves the lead arm to the contact arm in one write, so the
// one-subject CHECK on both tables holds throughout.
func TestAPromotionCarriesTheLeadsLinksAndBasesOntoTheContact(t *testing.T) {
	c := setupCarry(t)
	lead := c.lead(t, "Paul Promote", "paul@leadcarry.test", "Promote AG")
	token := c.leadHoldsLinkAndBasis(t, lead, "paul@leadcarry.test")

	contact, _, err := c.contacts.PromoteLead(c.admin, lead, contacts.PromoteLeadInput{
		Trigger: string(contacts.TriggerHumanQualify),
	})
	if err != nil {
		t.Fatalf("promoting: %v", err)
	}
	contactID := ids.UUID(contact.Id)

	if links, bases := c.subjectsHolding(t, "contact_id", contactID); links != 1 || bases != 1 {
		t.Errorf("the promoted contact holds %d link(s) and %d basis row(s), want 1 and 1", links, bases)
	}
	if links, bases := c.subjectsHolding(t, "lead_id", lead.UUID); links != 0 || bases != 0 {
		t.Errorf("%d link(s) and %d basis row(s) still name the promoted lead", links, bases)
	}
	ref, err := c.consent.ResolveWithdrawalToken(context.Background(), token)
	if err != nil || ref.ContactID.UUID != contactID {
		t.Errorf("the lead's unsubscribe link acts on %s (err=%v), want the new contact %s", ref.ContactID, err, contactID)
	}
}

// The review queue's merge disposition runs the lead merge inside its own
// transaction, so it carries the same rows.
func TestTheDedupeDispositionCarriesTheLosersLinks(t *testing.T) {
	c := setupCarry(t)
	winner := c.lead(t, "Mira Holt", "", "Holt & Co")
	loser := c.lead(t, "Mira Holtt", "mira@leadcarry.test", "Holt & Co")
	c.leadHoldsLinkAndBasis(t, loser, "mira@leadcarry.test")
	rows, _, err := c.contacts.ListDedupeCandidates(c.admin, contacts.DedupeQueueInput{EntityType: "lead"})
	if err != nil || len(rows) != 1 {
		t.Fatalf("the near-match pair did not reach the queue: %d row(s), err=%v", len(rows), err)
	}

	won := winner.UUID
	if _, err := c.contacts.DisposeDedupeCandidate(c.admin, rows[0].ID, "merge", &won); err != nil {
		t.Fatalf("disposing the pair as a merge: %v", err)
	}

	if links, bases := c.subjectsHolding(t, "lead_id", winner.UUID); links != 1 || bases != 1 {
		t.Errorf("the winning lead holds %d link(s) and %d basis row(s), want the loser's 1 and 1", links, bases)
	}
}

func TestAnUnwiredLeadMergeRefusesWhenALinkWouldBeStranded(t *testing.T) {
	c := setupCarry(t)
	unwired := contacts.NewStore(c.e.DB()).WithStopCarrier(c.consent)
	retired := c.lead(t, "Uwe Unwired", "uwe@leadcarry.test", "Unwired KG")
	survivor := c.lead(t, "Uwe Survivor", "uwe-survivor@leadcarry.test", "Unwired KG")
	c.leadHoldsLinkAndBasis(t, retired, "uwe@leadcarry.test")

	_, err := unwired.MergeLead(c.admin, retired, survivor)
	if _, ok := errors.AsType[*contacts.SatelliteCarrierNotWiredError](err); !ok {
		t.Fatalf("the lead merge answered %v, want SatelliteCarrierNotWiredError", err)
	}
	if links, bases := c.subjectsHolding(t, "lead_id", retired.UUID); links != 1 || bases != 1 {
		t.Errorf("the refused merge left %d link(s) and %d basis row(s) on the source, want 1 and 1", links, bases)
	}
}

// THE STOP LOCK COMES BEFORE THE LEAD ROW, in the lead merge and in promotion.
//
// Recording a stop takes consent's lock on the subject and then reads the
// subject row. A path that locks the row first and reaches for the stop lock
// later forms the other half of that cycle. This holds consent's lock through
// its own production method and asks the lock manager where the path parks:
// on the advisory lock, holding nothing on the lead table.
func TestEveryLeadMergePathTakesTheStopLockBeforeTheLeadRow(t *testing.T) {
	for _, path := range []struct {
		name string
		run  func(c *carryEnv, source, other ids.LeadID) error
	}{
		{"lead merge", func(c *carryEnv, source, other ids.LeadID) error {
			_, err := c.contacts.MergeLead(c.admin, source, other)
			return err
		}},
		{"promotion", func(c *carryEnv, source, _ ids.LeadID) error {
			_, _, err := c.contacts.PromoteLead(c.admin, source, contacts.PromoteLeadInput{
				Trigger: string(contacts.TriggerHumanQualify),
			})
			return err
		}},
	} {
		t.Run(path.name, func(t *testing.T) {
			c := setupCarry(t)
			source := c.lead(t, "Lock Source", "lock@leadcarry.test", "Lock GmbH")
			other := c.lead(t, "Lock Other", "lock-other@leadcarry.test", "Lock GmbH")
			holder, holderPID := c.holdStopLock(t, source)

			done := make(chan error, 1)
			go func() { done <- path.run(c, source, other) }()
			waiter := c.waitForAdvisoryWaiter(t, holder, holderPID, done)
			if held := leadLocksHeldBy(t, holder, waiter); len(held) > 0 {
				t.Fatalf("the %s waits on the stop lock while holding %v on the lead table — "+
					"it locked the row first, which is the order that deadlocks against recording a stop",
					path.name, held)
			}
			if err := holder.Rollback(context.Background()); err != nil {
				t.Fatalf("releasing the stop lock: %v", err)
			}
			select {
			case err := <-done:
				if err != nil {
					t.Fatalf("the %s failed once the lock was free: %v", path.name, err)
				}
			case <-time.After(30 * time.Second):
				t.Fatalf("the %s never finished after the lock was released", path.name)
			}
		})
	}
}

// holdStopLock opens a transaction holding consent's stop lock on one lead,
// taken through consent's own LockStopsTx so the key is the one production
// uses.
func (c *carryEnv) holdStopLock(t *testing.T, lead ids.LeadID) (pgx.Tx, int) {
	t.Helper()
	ctx := context.Background()
	holder, err := c.e.Pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := holder.Rollback(context.Background()); err != nil && !errors.Is(err, pgx.ErrTxClosed) {
			t.Errorf("releasing the lock holder: %v", err)
		}
	})
	var pid int
	if err := holder.QueryRow(ctx, `SELECT pg_backend_pid()`).Scan(&pid); err != nil {
		t.Fatal(err)
	}
	if err := c.consent.LockStopsTx(ctx, holder, commsauthz.LeadStopSubject(lead)); err != nil {
		t.Fatalf("taking the stop lock: %v", err)
	}
	return holder, pid
}

// waitForAdvisoryWaiter returns the backend queued on an advisory lock the
// holder has been granted. pg_locks reads the lock manager directly, so the
// look is current on every pass.
func (c *carryEnv) waitForAdvisoryWaiter(t *testing.T, holder pgx.Tx, holderPID int, done <-chan error) int {
	t.Helper()
	pace := time.NewTicker(20 * time.Millisecond)
	defer pace.Stop()
	budget := time.After(30 * time.Second)
	for {
		var waiter int
		err := holder.QueryRow(context.Background(), `
			SELECT w.pid FROM pg_locks w
			  JOIN pg_locks h
			    ON h.locktype = 'advisory' AND h.granted AND h.database = w.database
			   AND h.classid = w.classid AND h.objid = w.objid AND h.objsubid = w.objsubid
			 WHERE w.locktype = 'advisory' AND NOT w.granted AND h.pid = $1
			   AND w.database = (SELECT oid FROM pg_database WHERE datname = current_database())
			 LIMIT 1`, holderPID).Scan(&waiter)
		switch {
		case err == nil:
			return waiter
		case !errors.Is(err, pgx.ErrNoRows):
			t.Fatalf("reading the lock manager: %v", err)
		}
		select {
		case err := <-done:
			t.Fatalf("the path finished (%v) without waiting on the stop lock — it never took it", err)
		case <-budget:
			t.Fatal("nothing queued on the stop lock within 30s")
		case <-pace.C:
		}
	}
}

// leadLocksHeldBy names the row-level modes a backend holds on the lead table.
// AccessShareLock is a plain read and blocks nobody, so it is left out.
func leadLocksHeldBy(t *testing.T, probe pgx.Tx, pid int) []string {
	t.Helper()
	rows, err := probe.Query(context.Background(), `
		SELECT mode FROM pg_locks
		 WHERE pid = $1 AND locktype = 'relation' AND granted
		   AND relation = 'lead'::regclass AND mode <> 'AccessShareLock'`, pid)
	if err != nil {
		t.Fatalf("reading backend %d's locks on the lead table: %v", pid, err)
	}
	held, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		t.Fatalf("reading backend %d's locks on the lead table: %v", pid, err)
	}
	return held
}

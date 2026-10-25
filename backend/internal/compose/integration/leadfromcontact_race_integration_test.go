// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// Two creates from one contact, racing each other.
//
// `ensureContactNotWorked` locks the contact row before it probes. A second
// create queues on that row and reads the first lead once it commits.
//
// What the lock buys is the refusal's content, not the refusal.
// `uq_lead_from_contact_live` rejects the loser either way, as a unique
// violation that names no lead. A screen that asked to work this contact could
// not then open the one already being worked.
//
// So the assertion is the named id, and the contact carries no email and no
// LinkedIn profile. Those have refusals of their own, which would answer first
// and leave the contact key untested.
//
// Both creates are parked on a pinned contact row before either is released.
// The race is what the test arranges, not something a run may happen to hit. A
// round where the two never met would pass while proving nothing.

import (
	"context"
	"errors"
	"sync"
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/testdb"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// racers is two because two is what the lock orders. A third would queue behind
// the same row and say nothing the second does not.
const racers = 2

func TestTwoCreatesFromOneContactEndInOneLeadNamedByTheRefusal(t *testing.T) {
	e := Setup(t)
	contact := bareContact(t, e, "Nadia Unreachable")

	release, holderPID := pinRow(t, holdContactRow, contact.UUID)

	type attempt struct {
		lead crmcontracts.Lead
		err  error
	}
	done := make(chan attempt, racers)
	firstReturned := make(chan struct{})
	// Closed once, by whichever racer returns first: two landing together would
	// close it twice and panic, and the panic would read as a product defect.
	var announceReturn sync.Once
	for range racers {
		go func() {
			lead, _, err := e.Contacts.CreateLead(e.Admin(), contacts.CreateLeadInput{
				FullName:      new("Nadia Unreachable"),
				Source:        "manual",
				OwnerID:       userIDPtr(&e.Rep1),
				FromContactID: &contact,
			})
			announceReturn.Do(func() { close(firstReturned) })
			done <- attempt{lead, err}
		}()
	}

	waitUntilQueueIs(t, racers, holderPID, firstReturned,
		"a create returned without ever waiting on the contact row — it does not take the "+
			"contact lock, so nothing here orders two creates against each other",
		"the creates never both queued behind the contact lock: one of them reads that row "+
			"without holding it, and the refusal it writes cannot name the lead that won")
	release()

	first, second := <-done, <-done
	won, lost := first, second
	if won.err != nil {
		won, lost = second, first
	}
	if won.err != nil {
		t.Fatalf("both creates were refused, so no lead was written at all: %v / %v",
			first.err, second.err)
	}
	if lost.err == nil {
		t.Fatalf("both creates wrote a lead from one contact: %s and %s",
			won.lead.Id, lost.lead.Id)
	}
	var duplicate *contacts.DuplicateContactLeadError
	if !errors.As(lost.err, &duplicate) {
		t.Fatalf("the losing create was refused as %v, want a duplicate-contact refusal that "+
			"names the lead it lost to", lost.err)
	}
	if duplicate.ExistingID.UUID != ids.UUID(won.lead.Id) {
		t.Fatalf("the refusal names lead %s and the create that won wrote %s",
			duplicate.ExistingID, won.lead.Id)
	}
}

// bareContact is a contact with no email and no LinkedIn profile: the contact
// key is the only thing a second lead can be refused over.
func bareContact(t *testing.T, e *Env, name string) ids.ContactID {
	t.Helper()
	contact, err := e.Contacts.CreateContact(e.Admin(), contacts.CreateContactInput{
		Acquisition: contacts.Acquisition{Kind: contacts.AcquiredReferral},
		FullName:    name,
	})
	if err != nil {
		t.Fatalf("seeding the contact two creates contend over: %v", err)
	}
	return ids.From[ids.ContactKind](ids.UUID(contact.Id))
}

// waitersBehind counts the backends queued on a row the given backend holds,
// including the ones queued behind each other.
//
// `pg_blocking_pids` reports only the holders ahead of a waiter in the lock
// queue. The second create is therefore blocked by the first create rather than
// by the pin.
//
// A probe counting direct waiters alone stops at one however many are queued.
// The wait then runs until the app's statement timeout cancels the creates,
// which is a pass-shaped failure. The racers are gone, and the probe reports
// that it never saw contention.
//
// Two levels is what two racers need. A third would want the recursive walk,
// and `racers` is fixed at two for the reason stated there.
const waitersBehind = `
	WITH blocked AS (
	  SELECT a.pid, pg_blocking_pids(a.pid) AS blockers FROM pg_stat_activity a
	)
	SELECT count(*) FROM blocked b
	 WHERE $1 = ANY (b.blockers)
	    OR EXISTS (SELECT 1 FROM blocked ahead
	                WHERE ahead.pid = ANY (b.blockers)
	                  AND $1 = ANY (ahead.blockers))`

// waitUntilQueueIs waits until want backends are queued on the pinned row.
//
// Both writers have to be parked before the row is released. Releasing with one
// still in flight runs the second against a lead already committed, which is a
// sequential write and not a race.
func waitUntilQueueIs(t *testing.T, want int, holder uint32, racerReturned <-chan struct{},
	finishedEarly, missed string,
) {
	t.Helper()
	conn := freshConnection(t)
	testdb.WaitForContention(t, racerReturned, finishedEarly, missed,
		func(ctx context.Context) (bool, error) {
			// pg_stat_activity is materialized once per transaction and cached,
			// so a probe that skips this clear answers from its first look.
			if _, err := conn.Exec(ctx, `SELECT pg_stat_clear_snapshot()`); err != nil {
				return false, err
			}
			var queued int
			err := conn.QueryRow(ctx, waitersBehind, holder).Scan(&queued)
			return queued >= want, err
		})
}

// A create and a demote over one contact take its rows in the same order.
//
// The demote goes contact then lead, which
// TestTheDemoteTakesTheContactBeforeTheLead holds. A create reaching the lead
// first would close the cycle: each side holds what the other waits for.
// Postgres aborts one with 40P01, and a caller sees a 5xx where a serialized
// race owes them a refusal.
//
// What is asserted is the order, not the absence of a deadlock. Racing the two
// and watching for 40P01 passes on every run where they never interleaved.
//
// So the create is parked on a pinned contact row and asked what it holds. It
// holds nothing on the lead, or it does not take the contact first.
func TestACreateFromAContactTakesTheContactBeforeTheLead(t *testing.T) {
	e := Setup(t)
	contact := bareContact(t, e, "Teodor Parked")
	worked, _, err := e.Contacts.CreateLead(e.Admin(), contacts.CreateLeadInput{
		FullName: new("Teodor Parked"), Source: "manual",
		OwnerID: userIDPtr(&e.Rep1), FromContactID: &contact,
	})
	if err != nil {
		t.Fatalf("seeding the lead the contact is already worked through: %v", err)
	}

	release, holderPID := pinRow(t, holdContactRow, contact.UUID)
	refusal := make(chan error, 1)
	createReturned := make(chan struct{})
	go func() {
		defer close(createReturned)
		_, _, err := e.Contacts.CreateLead(e.Admin(), contacts.CreateLeadInput{
			FullName: new("Teodor Again"), Source: "manual",
			OwnerID: userIDPtr(&e.Rep1), FromContactID: &contact,
		})
		refusal <- err
	}()

	waitUntilQueueIs(t, 1, holderPID, createReturned,
		"the create returned without ever waiting on the contact row — it does not take that "+
			"lock, so nothing orders it against a demote that does",
		"the create never queued behind the contact lock: it reads that row without holding "+
			"it, and a demote holding the contact can then meet it on the lead")

	held, err := lockIsRefused(t, probeLeadForUpdate, ids.UUID(worked.Id))
	if err != nil {
		t.Fatalf("asking what the parked create holds on the lead: %v", err)
	}
	if held {
		t.Fatal("the create is waiting on the contact while already holding the lead, which " +
			"is the half of a deadlock a demote going contact then lead closes")
	}
	release()

	var duplicate *contacts.DuplicateContactLeadError
	if got := <-refusal; !errors.As(got, &duplicate) {
		t.Fatalf("the create past the released contact lock answered %v, want the "+
			"duplicate-contact refusal", got)
	}
}

// A create and a demote over one contact, both in flight, end without a cycle.
//
// The two take the contact and the lead in the same order, which the tests above
// and TestTheDemoteTakesTheContactBeforeTheLead each hold for one side. Neither
// runs the two together, so nothing yet says what happens when they do.
//
// Both are parked on the pinned contact row before it is released, so the
// interleaving is arranged rather than hoped for.
//
// What is asserted is that neither comes back with 40P01. A cycle aborts one of
// them, and its caller reads a 5xx where a serialized race owes them an answer.
func TestACreateAndADemoteOverOneContactDoNotDeadlock(t *testing.T) {
	e := Setup(t)
	lead, contact := promotedPair(t, e, "Ilse Verwaltung", "ilse@kanzlei.test")

	release, holderPID := pinRow(t, holdContactRow, contact.UUID)
	type outcome struct {
		writer string
		err    error
	}
	outcomes := make(chan outcome, racers)
	var announceReturn sync.Once
	firstReturned := make(chan struct{})
	report := func(writer string, err error) {
		announceReturn.Do(func() { close(firstReturned) })
		outcomes <- outcome{writer, err}
	}

	go func() {
		_, err := e.Contacts.DemoteLead(e.Admin(), lead, "not a real opportunity")
		report("demote", err)
	}()
	go func() {
		_, _, err := e.Contacts.CreateLead(e.Admin(), contacts.CreateLeadInput{
			FullName: new("Ilse Again"), Source: "manual",
			OwnerID: userIDPtr(&e.Rep1), FromContactID: &contact,
		})
		report("create", err)
	}()

	waitUntilQueueIs(t, racers, holderPID, firstReturned,
		"one of the two returned without ever waiting on the contact row, so they were never "+
			"ordered against each other and a cycle between them stays untested",
		"the create and the demote never both queued behind the contact lock: one of them "+
			"reaches its second row without holding the contact, which is the order a cycle needs")
	release()

	// Which of the two wins the row is the lock queue's business. The outcomes
	// are read per writer and neither order is required.
	//
	// What both orders owe is an answer. A cycle aborts one with 40P01, and a
	// stopped statement carries 57014. Either leaves a caller a 5xx where a
	// serialized race owes them a refusal or a lead.
	//
	// Whichever runs second re-reads the contact under the same lock. So the
	// demote-first order refuses the create by ensureContactStillLive, which is
	// the shape seen here and not a requirement.
	settled := 0
	for range racers {
		got := <-outcomes
		switch {
		case storekit.IsDeadlock(got.err):
			t.Fatalf("the %s was aborted in a cycle with the other writer: %v", got.writer, got.err)
		case storekit.IsQueryCanceled(got.err):
			t.Fatalf("the %s had its statement stopped rather than being serialized: %v",
				got.writer, got.err)
		case got.err == nil:
			settled++
		}
	}
	if settled == 0 {
		t.Fatal("neither the create nor the demote came through, so the two did not serialize: " +
			"one of them owes an answer whichever took the contact first")
	}
}

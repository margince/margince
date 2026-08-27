// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// The lock ORDER a demote and a merge take over the same two rows.
//
// Both writers touch a lead and the person it was promoted into. The merge
// locks the two people (LockPair) and then repoints lead.promoted_person_id,
// so it goes person -> lead. The demote used to go lead -> person, and two
// writers taking the same pair in opposite orders is the whole of a deadlock:
// each holds what the other is waiting for, Postgres picks one and aborts it
// with 40P01, and the caller sees a 5xx where the losing side of a serialized
// race should see a clean refusal.
//
// What is asserted is the ORDER, not the absence of the deadlock. Racing a
// demote against a merge and watching for 40P01 proves nothing on a run where
// the two never interleaved, and a green suite that only means "they missed
// each other today" is the shape AGENTS.md rule 8 calls a census that already
// failed. So each round PINS one row open from this file's own connection and
// asks what the parked writer is holding — a question with the same answer on
// every machine and on every scheduling.
//
// Both halves are needed, and each fails against a different mutation:
//
//   - lead-not-held (round one) fails if the order goes back to lead-first:
//     the demote would sit on the person's lock while holding the lead, which
//     is exactly the state the merge deadlocks against;
//   - person-held (round two) fails if the person lock is dropped entirely.
//     An order nothing takes is not an order, and lead-not-held alone reads
//     green over that.

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/margince/margince/backend/internal/modules/people"
	"github.com/margince/margince/backend/internal/platform/testdb"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// The rows a round pins, and the probes that ask what the parked demote holds.
const (
	holdPersonRow = `SELECT 1 FROM person WHERE id = $1 FOR UPDATE`
	holdLeadRow   = `SELECT 1 FROM lead WHERE id = $1 FOR UPDATE`

	probeLeadForUpdate   = `SELECT 1 FROM lead WHERE id = $1 FOR UPDATE NOWAIT`
	probePersonForUpdate = `SELECT 1 FROM person WHERE id = $1 FOR UPDATE NOWAIT`
)

func TestTheDemoteTakesThePersonBeforeTheLead(t *testing.T) {
	e := Setup(t)

	t.Run("parked on the person, it is not holding the lead", func(t *testing.T) {
		theDemoteWaitsForThePersonEmptyHanded(t, e)
	})
	t.Run("parked on the lead, it is already holding the person", func(t *testing.T) {
		theDemoteReachesTheLeadHoldingThePerson(t, e)
	})
}

// theDemoteWaitsForThePersonEmptyHanded pins the person — the row the merge
// takes first — and asserts that a demote queued behind it holds no lead.
//
// This is the deadlock ingredient stated directly. A demote parked here while
// holding the lead is one half of the cycle; a merge that has the person and
// wants the lead is the other, and it is the merge's own order that makes it
// so. With the demote empty-handed, that merge finishes and the demote then
// re-reads what the merge left.
func theDemoteWaitsForThePersonEmptyHanded(t *testing.T, e *Env) {
	lead, person := promotedPair(t, e, "Ada Parked", "ada.parked@prospect.test")

	holder, holderPID := pinRow(t, holdPersonRow, person.UUID)

	demoted := make(chan error, 1)
	demoteReturned := make(chan struct{})
	go func() {
		defer close(demoteReturned)
		_, err := e.People.DemoteLead(e.Admin(), lead, "not a real opportunity")
		demoted <- err
	}()

	waitUntilSomethingQueuesBehind(t, holderPID, demoteReturned,
		"the demote returned without ever waiting on the promoted person — it does not lock the row "+
			"the merge path locks first, so nothing here orders the two against each other",
		"the demote never queued behind the person lock: it either reads that row without holding it, "+
			"or it takes some other row first, and either way the merge's order is not the order it takes")

	// The question this round exists to ask. A refusal here means the demote is
	// holding the lead while it waits for the person — lead-then-person, the
	// order that deadlocks against the merge.
	if held, err := lockIsRefused(t, probeLeadForUpdate, lead.UUID); err != nil {
		t.Fatalf("asking whether the parked demote holds the lead: %v", err)
	} else if held {
		t.Fatal("the demote is waiting on the promoted person while already holding the lead. " +
			"MergePerson locks the person first and then writes the lead, so those two orders " +
			"close a cycle: each writer ends up holding what the other is waiting for, and " +
			"Postgres resolves it by aborting one of them with a deadlock")
	}

	holder()
	if err := <-demoted; err != nil {
		t.Fatalf("the demote never recovered from the wait it was made to serve: %v", err)
	}
}

// theDemoteReachesTheLeadHoldingThePerson pins the LEAD and asserts the person
// is already locked by the time the demote gets there.
//
// Without this half, deleting the person lock outright passes round one: a
// demote that never touches the person is trivially "not holding the lead
// while waiting for it", because it never waits. An order is only an order if
// both locks are taken.
func theDemoteReachesTheLeadHoldingThePerson(t *testing.T, e *Env) {
	lead, person := promotedPair(t, e, "Bo Parked", "bo.parked@prospect.test")

	holder, holderPID := pinRow(t, holdLeadRow, lead.UUID)

	demoted := make(chan error, 1)
	demoteReturned := make(chan struct{})
	go func() {
		defer close(demoteReturned)
		_, err := e.People.DemoteLead(e.Admin(), lead, "not a real opportunity")
		demoted <- err
	}()

	waitUntilSomethingQueuesBehind(t, holderPID, demoteReturned,
		"the demote returned without ever waiting on the lead — it wrote the lead row without "+
			"holding it, which is a different defect from the one this file is about",
		"the demote never queued behind the lead lock")

	if held, err := lockIsRefused(t, probePersonForUpdate, person.UUID); err != nil {
		t.Fatalf("asking whether the parked demote holds the person: %v", err)
	} else if !held {
		t.Fatal("the demote reached the lead without holding the promoted person. It takes no lock " +
			"on the row it is about to unwind, so a merge can repoint that person underneath it — " +
			"and an order neither writer takes cannot order them")
	}

	holder()
	if err := <-demoted; err != nil {
		t.Fatalf("the demote never recovered from the wait it was made to serve: %v", err)
	}
}

// promotedPair seeds a lead and promotes it, returning the two rows the demote
// and the merge contend over.
func promotedPair(t *testing.T, e *Env, name, email string) (ids.LeadID, ids.PersonID) {
	t.Helper()
	lead := seedLead(t, e, name, email, &e.Rep1)
	person, merged, err := e.People.PromoteLead(e.Admin(), lead, people.PromoteLeadInput{
		Trigger: "inbound_reply", EvidenceNote: strPtr("replied to outreach"),
	})
	if err != nil {
		t.Fatalf("promoting %s so there is a person to contend over: %v", name, err)
	}
	if merged {
		t.Fatalf("%s promoted into an existing person, so this round would contend over a row "+
			"another round is also using", name)
	}
	return lead, ids.From[ids.PersonKind](ids.UUID(person.Id))
}

// pinRow holds one row FOR UPDATE until the returned release runs, and reports
// the backend holding it so a probe can ask who is queued behind that pid
// rather than who is queued behind anything at all.
//
// The release is idempotent and also registered as cleanup: a round that fails
// an assertion returns without reaching its own release, and a transaction
// still holding a row outlives the test that opened it.
func pinRow(t *testing.T, query string, id ids.UUID) (func(), uint32) {
	t.Helper()
	ctx := context.Background()
	conn := freshConnection(t)
	tx, err := conn.Begin(ctx)
	if err != nil {
		t.Fatalf("opening the transaction that pins the contested row: %v", err)
	}
	if _, err := tx.Exec(ctx, query, id); err != nil {
		t.Fatalf("pinning the contested row: %v", err)
	}
	released := false
	release := func() {
		if released {
			return
		}
		released = true
		if err := tx.Rollback(ctx); err != nil && !errors.Is(err, pgx.ErrTxClosed) {
			t.Errorf("releasing the contested row: %v", err)
		}
	}
	t.Cleanup(release)
	return release, conn.PgConn().PID()
}

// waitUntilSomethingQueuesBehind waits until a backend is blocked by holder,
// which is what proves the demote reached the pinned row instead of finishing
// past it or stopping somewhere else.
//
// It asks pg_blocking_pids rather than "is anything waiting anywhere": the lane
// runs packages concurrently against their own databases, and an unrelated
// waiter would otherwise certify a round in which the demote never arrived.
func waitUntilSomethingQueuesBehind(t *testing.T, holder uint32, demoteReturned <-chan struct{}, finishedEarly, missed string) {
	t.Helper()
	conn := freshConnection(t)
	testdb.WaitForContention(t, demoteReturned, finishedEarly, missed, func(ctx context.Context) (bool, error) {
		// pg_stat_activity's row set is materialized once per transaction and
		// cached until something drops it, so a probe that skips this clear
		// answers from the snapshot its first look took — and the demote's
		// connection may have been dialled after that.
		if _, err := conn.Exec(ctx, `SELECT pg_stat_clear_snapshot()`); err != nil {
			return false, err
		}
		var queued bool
		err := conn.QueryRow(ctx,
			`SELECT EXISTS (SELECT 1 FROM pg_stat_activity a
			  WHERE $1 = ANY (pg_blocking_pids(a.pid)))`, holder).Scan(&queued)
		return queued, err
	})
}

// lockIsRefused answers whether the row is already held hard enough to refuse
// this probe. The probe's own lock lives for the statement and no longer, so
// asking never changes which side of the race wins.
func lockIsRefused(t *testing.T, query string, id ids.UUID) (bool, error) {
	t.Helper()
	_, err := freshConnection(t).Exec(context.Background(), query, id)
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == lockNotAvailable {
		return true, nil
	}
	return false, err
}

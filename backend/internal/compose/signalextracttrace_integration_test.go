// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// The trace's per-conversation read, against the same database the pass runs
// against.
//
// It cannot be a unit test: what it asserts is that the arms composed into
// threadOfferQuery answer the same as the arms composed into dueThreadsQuery,
// and both are SQL. A Go double would prove that two Go structs agree.
//
// So each case seeds a conversation, asks BOTH — is this thread on the queue,
// and what does the trace say about it — and requires the two to agree. A
// divergence here is the defect the shared arms exist to prevent, arriving as a
// member being told one thing about a conversation the pass treated as another.

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// traceReadClock is fixed so "settled" is a property of the fixture rather than
// of how long the test took.
var traceReadClock = time.Date(2026, 6, 4, 9, 0, 0, 0, time.UTC)

// seedConversation writes one connector email on a thread, filed against a
// contact who works at the account.
//
// Filed against the CONTACT and never against the company, because that is what
// capture writes: an account is reached through its people. A fixture with a
// direct company link would describe a row no connector produces, and the arm
// under test — reaches exactly one account — would be satisfied by the wrong
// shape.
func seedConversation(
	t *testing.T, e *integration.Env, company ids.UUID, key string, at time.Time, audience string,
) ids.UUID {
	t.Helper()
	owner := integration.OwnerConn(t)
	contact := e.SeedContact(t, key+" contact", &e.Rep1)
	if _, err := owner.Exec(context.Background(),
		`INSERT INTO relationship (kind, contact_id, company_id, source, captured_by)
		 VALUES ('employment', $1, $2, 'manual', 'human:x')`, contact, company); err != nil {
		t.Fatal(err)
	}
	id := ids.NewV7()
	if _, err := owner.Exec(context.Background(), `
		INSERT INTO activity (id, kind, direction, subject, body, thread_key,
		                      occurred_at, created_at, source, captured_by, audience)
		VALUES ($1, 'email', 'inbound', 'Renewal', 'Happy to continue.', $2, $3, $3,
		        'gmail', 'connector:gmail', $4)`,
		id, key, at, audience); err != nil {
		t.Fatal(err)
	}
	integration.LinkActivity(t, owner, id, "contact", contact)
	return id
}

// offeredKeys collects the threads the pass would read now, as a set.
func offeredKeys(t *testing.T, e *integration.Env) map[string]bool {
	t.Helper()
	out := map[string]bool{}
	as := e.Admin()
	if err := database.WithWorkspaceTx(as, e.Pool, func(tx pgx.Tx) error {
		due, err := dueThreads(as, tx, traceReadClock, 100)
		if err != nil {
			return err
		}
		for _, thread := range due {
			out[thread.Key] = true
		}
		return nil
	}); err != nil {
		t.Fatalf("listing the due threads: %v", err)
	}
	return out
}

// A settled conversation on one account is on the queue, and the trace says
// every arm of the offer is satisfied. Both halves matter: the second without
// the first would pass over a fixture the pass ignores.
func TestTheTraceAgreesWithTheQueueOnAConversationItWouldRead(t *testing.T) {
	e := integration.Setup(t)
	company := e.SeedCompany(t, "Acme", &e.Rep1)
	const key = "thread-trace-offered"
	activity := seedConversation(t, e, company, key, traceReadClock.Add(-48*time.Hour), "workspace")

	if !offeredKeys(t, e)[key] {
		t.Fatal("the fixture is not on the queue, so nothing below is comparing the two readings")
	}

	reading, err := NewThreadReadings(e.DB(), func() time.Time { return traceReadClock }).
		ReadThread(e.Admin(), key, activity.String())
	if err != nil {
		t.Fatalf("reading the conversation: %v", err)
	}
	if !reading.Known {
		t.Fatal("the trace sees no conversation on a thread the queue just offered")
	}
	for name, satisfied := range map[string]bool{
		"reaches one account": reading.ReachesOneAccount,
		"one body of work":    reading.IsOneBodyOfWork,
		"fully open":          reading.IsFullyOpen,
		"has a named reader":  reading.HasANamedReader,
		"has settled":         reading.HasSettled,
		"has moved":           reading.HasMoved,
	} {
		if !satisfied {
			t.Errorf("the trace reports %q unsatisfied on a conversation the queue offered", name)
		}
	}
	if reading.IsParked || reading.Scanned || reading.Cited {
		t.Errorf("a never-read conversation reports parked=%v scanned=%v cited=%v",
			reading.IsParked, reading.Scanned, reading.Cited)
	}
}

// A conversation still moving is off the queue, and the trace names THAT arm
// rather than reporting it as unread — which is the whole difference between a
// state and a silence.
func TestTheTraceNamesTheArmThatKeptAConversationOffTheQueue(t *testing.T) {
	e := integration.Setup(t)
	company := e.SeedCompany(t, "Acme", &e.Rep1)
	const key = "thread-trace-moving"
	// One minute old against a six-hour settle window.
	activity := seedConversation(t, e, company, key, traceReadClock.Add(-time.Minute), "workspace")

	if offeredKeys(t, e)[key] {
		t.Fatal("a conversation a minute old was offered; the settle window is not in force in this fixture")
	}

	reading, err := NewThreadReadings(e.DB(), func() time.Time { return traceReadClock }).
		ReadThread(e.Admin(), key, activity.String())
	if err != nil {
		t.Fatalf("reading the conversation: %v", err)
	}
	if !reading.Known {
		t.Fatal("the trace sees no conversation on a thread that has one")
	}
	if reading.HasSettled {
		t.Error("the trace reports a conversation settled that the queue refused as still moving")
	}
	if !reading.ReachesOneAccount {
		// The arms are independent, and a fixture that failed two of them would
		// let the rung's first-refusal order hide which one this test is about.
		t.Error("the fixture fails a second arm as well, so it cannot isolate the settle window")
	}
}

// A message limited from the summary's readers takes its thread off the queue,
// and the trace names that arm. This is the one refusal a member is most likely
// to ask about, because nothing else on the screen explains it.
func TestTheTraceNamesALimitedMessageAsTheRefusal(t *testing.T) {
	e := integration.Setup(t)
	company := e.SeedCompany(t, "Acme", &e.Rep1)
	const key = "thread-trace-limited"
	at := traceReadClock.Add(-48 * time.Hour)
	activity := seedConversation(t, e, company, key, at, "workspace")
	seedConversation(t, e, company, key, at.Add(time.Minute), "participants")

	if offeredKeys(t, e)[key] {
		t.Fatal("a thread carrying a limited message was offered")
	}

	reading, err := NewThreadReadings(e.DB(), func() time.Time { return traceReadClock }).
		ReadThread(e.Admin(), key, activity.String())
	if err != nil {
		t.Fatalf("reading the conversation: %v", err)
	}
	if reading.IsFullyOpen {
		t.Error("the trace reports a thread fully open that the queue refused for a limited message")
	}
}

// A thread key nothing was captured under is not a conversation, and the read
// says so rather than answering about an empty aggregate.
func TestATraceReadOfAThreadWithNoConversationIsNotKnown(t *testing.T) {
	e := integration.Setup(t)
	reading, err := NewThreadReadings(e.DB(), func() time.Time { return traceReadClock }).
		ReadThread(e.Admin(), "thread-that-never-existed", ids.NewV7().String())
	if err != nil {
		t.Fatalf("reading the conversation: %v", err)
	}
	if reading.Known {
		t.Error("the trace claims to know a conversation that was never captured")
	}
}

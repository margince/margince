// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package consent

// An "unsubscribe from everything" pressed while a grant is landing.
//
// The press used to read the recipient's purposes in one transaction and
// withdraw them in another. A purpose granted between the two — a confirmation
// round-trip arriving on the press — was not in the selection, survived the
// press, and the response reported success. The recipient went on receiving one
// lane after being told they were done.
//
// The window is real but small, so this test does not race for it: it holds the
// grant open in its own transaction and lets the press meet the lock. That is
// deterministic — the press blocks or it does not — and it is the same
// interleaving, made observable.
//
// Two things have to be true for it to close, and each is defeated on its own:
// the press has to choose its purposes inside the transaction that withdraws
// them, and the per-purpose write has to LOCK the row it reads before deciding
// there is nothing to do. Without the second, the press reads the state its own
// transaction started with, answers no-op, and the grant stands.

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestUnsubscribeAllStopsAPurposeGrantedWhileItWasRunning(t *testing.T) {
	e := setupChannelConsent(t)
	token := seedPreferenceToken(t, e)
	ctx := context.Background()

	// The recipient has already opted out of this lane — through the endpoint
	// that does it, so the row is the one production writes. That state is what
	// made the old selection skip the lane, and what a confirmation landing now
	// would undo.
	newsletter := "newsletter"
	pressUnsubscribe(t, e, token, &newsletter, "List-Unsubscribe=One-Click")
	if got := consentStateOf(t, e, e.newsletter); got != string(StateWithdrawn) {
		t.Fatalf("precondition: newsletter = %q, want withdrawn", got)
	}

	// The grant, held open. Committed only once the press is waiting on it, so
	// the press's own view of this row is stale by exactly the window the
	// defect lived in.
	granting, err := e.owner.Begin(ctx)
	if err != nil {
		t.Fatalf("opening the granting transaction: %v", err)
	}
	defer func() {
		if err := granting.Rollback(context.Background()); err != nil && !errors.Is(err, pgx.ErrTxClosed) {
			t.Errorf("rolling back the granting transaction: %v", err)
		}
	}()
	if _, err := granting.Exec(ctx,
		`UPDATE contact_consent SET state = 'granted'
		  WHERE contact_id = $1 AND purpose_id = $2`, e.contact, e.newsletter); err != nil {
		t.Fatalf("granting: %v", err)
	}

	pressed := make(chan []string, 1)
	go func() {
		pressed <- pressUnsubscribe(t, e, token, nil, "")
	}()

	waitUntilBlockedBy(t, e.owner)
	if err := granting.Commit(ctx); err != nil {
		t.Fatalf("committing the grant: %v", err)
	}

	select {
	case stopped := <-pressed:
		var sawNewsletter bool
		for _, key := range stopped {
			if key == "newsletter" {
				sawNewsletter = true
			}
		}
		if !sawNewsletter {
			t.Errorf("the press reported %v — the lane granted while it ran is the one it must stop", stopped)
		}
	case <-time.After(30 * time.Second):
		t.Fatal("the press never returned after the grant committed — it is waiting on a lock nothing releases")
	}

	if got := consentStateOf(t, e, e.newsletter); got != string(StateWithdrawn) {
		t.Errorf("newsletter = %q, want withdrawn — a grant that landed DURING an "+
			"unsubscribe-from-everything survived it, and the recipient was told they were done", got)
	}
}

// waitUntilBlockedBy blocks until some session is waiting on a lock THIS
// connection holds.
//
// Asked as "who is blocking whom" rather than "is anyone waiting", and the
// difference decides whether the case tests anything: a count of every session
// with wait_event_type = 'Lock' returns for a wait belonging to some other test
// in the same database, the holder then commits before the press has reached
// the row, and the case passes having interleaved nothing — which is exactly
// what its own failure message warns about.
//
// pg_blocking_pids answers precisely, and covers both shapes this file needs:
// the row lock a held transaction takes, and the advisory lock a session takes.
//
// Asked of the DATABASE rather than timed: what the test needs to know is that
// the press has reached the lock and stopped there, and Postgres reports
// exactly that. A sleep would either be too short — committing before the press
// arrives, so nothing was interleaved — or slow for everybody. The small pause
// between asks is not the wait itself; it stops the poll competing with the
// press for a pool connection.
func waitUntilBlockedBy(t *testing.T, holder *pgx.Conn) {
	t.Helper()
	waitUntilNBlockedBy(t, holder, 1)
}

// waitUntilNBlockedBy is the same wait for a known NUMBER of waiters.
//
// One is the common case and reads better as its own name. A caller arranging
// an interleaving needs more: with two dispatches meant to meet on one lock,
// waiting for "somebody is blocked" returns on the FIRST of them and releases
// the lock while the second may not have run a statement yet — so the two race
// exactly as the test was written to prevent. Asking for the count is what makes
// the arrangement observed rather than assumed.
func waitUntilNBlockedBy(t *testing.T, holder *pgx.Conn, want int) {
	t.Helper()
	ctx := context.Background()
	var holderPID int
	if err := holder.QueryRow(ctx, `SELECT pg_backend_pid()`).Scan(&holderPID); err != nil {
		t.Fatalf("reading the holding connection's pid: %v", err)
	}
	deadline := time.Now().Add(30 * time.Second)
	for {
		// pg_stat_activity is materialized once per transaction and cached
		// until it ends, so a probe that did not clear it cannot see a backend
		// that dialled after the snapshot was taken — the wait then runs to its
		// deadline over contention that is really there.
		if _, err := holder.Exec(ctx, `SELECT pg_stat_clear_snapshot()`); err != nil {
			t.Fatalf("clearing the stats snapshot before probing: %v", err)
		}
		var waiting int
		if err := holder.QueryRow(ctx,
			`SELECT count(*) FROM pg_stat_activity WHERE $1 = ANY(pg_blocking_pids(pid))`,
			holderPID).Scan(&waiting); err != nil {
			t.Fatalf("reading who this connection is blocking: %v", err)
		}
		if waiting >= want {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("%d backend(s) ever waited on the lock this connection holds, want %d: the "+
				"press did not reach it, so this case interleaved nothing and would pass over "+
				"the defect it is named for", waiting, want)
		}
		//craft:ignore test-sleep the wait IS on the condition — pg_blocking_pids, asked in the loop above. This paces the asking so the poll does not compete with the press for a pool connection, which CodeRabbit raised on #4631; removing it makes the test flakier, not less.
		time.Sleep(20 * time.Millisecond)
	}
}

// Two multi-purpose consent transactions for one contact do not interleave.
//
// Each of the three writes several purposes in one transaction, and each takes
// its row locks in its OWN order: the withdrawal sweeps go by ascending purpose
// key, a granular save goes withdrawals-first so a refused grant cannot cost
// the suppression beside it. A save of {grant a, withdraw b} therefore locks b
// before a while an stop-all-marketing locks a before b — and two at once
// on one contact deadlock. Postgres aborts one, and the reader sees a preference
// change that failed for no reason they can act on.
//
// Held by taking the contact's lock from another session and watching the write
// wait for it. That is the mechanism itself rather than a race for the symptom:
// a deadlock test would have to lose a coin toss to fail.
func TestOneContactsConsentWritesDoNotInterleave(t *testing.T) {
	e := setupChannelConsent(t)
	ctx := context.Background()

	holder, err := pgx.Connect(ctx, os.Getenv("MARGINCE_TEST_DSN"))
	if err != nil {
		t.Fatalf("opening the holding connection: %v", err)
	}
	t.Cleanup(func() {
		if err := holder.Close(context.Background()); err != nil {
			t.Errorf("closing the holding connection: %v", err)
		}
	})
	// The same key the writes take, spelled the same way: a lock on some other
	// number would be a test of nothing.
	if _, err := holder.Exec(ctx,
		`SELECT pg_advisory_lock(hashtextextended($1::text, 0))`, e.contact); err != nil {
		t.Fatalf("taking the contact's lock: %v", err)
	}

	pressed := make(chan error, 1)
	go func() {
		_, err := e.store.PublicStopAllMarketing(publicPreferencesCtx(e), e.contact)
		pressed <- err
	}()

	waitUntilBlockedBy(t, holder)
	if _, err := holder.Exec(ctx,
		`SELECT pg_advisory_unlock(hashtextextended($1::text, 0))`, e.contact); err != nil {
		t.Fatalf("releasing the contact's lock: %v", err)
	}

	select {
	case err := <-pressed:
		if err != nil {
			t.Fatalf("the press failed once the lock was free: %v", err)
		}
	case <-time.After(30 * time.Second):
		t.Fatal("the press never finished after the lock was released")
	}
}

// publicPreferencesCtx is the principal the public middleware binds on this
// surface: a system actor, because the caller holds a token rather than a seat.
func publicPreferencesCtx(e *channelConsentEnv) context.Context {
	ctx := principal.WithWorkspaceID(context.Background(), e.ws)
	ctx = principal.WithCorrelationID(ctx, ids.NewV7())
	return principal.WithActor(ctx, principal.Principal{
		Type: principal.PrincipalSystem, ID: "system:public_preferences",
	})
}

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
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
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
		`UPDATE person_consent SET state = 'granted'
		  WHERE person_id = $1 AND purpose_id = $2`, e.person, e.newsletter); err != nil {
		t.Fatalf("granting: %v", err)
	}

	pressed := make(chan []string, 1)
	go func() {
		pressed <- pressUnsubscribe(t, e, token, nil, "")
	}()

	waitForLockWait(t, e)
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

// waitForLockWait blocks until some other session is waiting on a lock.
//
// Asked of the DATABASE rather than timed: what the test needs to know is that
// the press has reached the row and stopped there, and Postgres reports exactly
// that. A sleep would either be too short — committing before the press arrives,
// so nothing was interleaved and the case proves nothing — or slow for
// everybody.
func waitForLockWait(t *testing.T, e *channelConsentEnv) {
	t.Helper()
	deadline := time.Now().Add(30 * time.Second)
	for {
		var waiting int
		if err := e.owner.QueryRow(context.Background(),
			`SELECT count(*) FROM pg_stat_activity
			  WHERE datname = current_database() AND wait_event_type = 'Lock'`).Scan(&waiting); err != nil {
			t.Fatalf("reading the lock waits: %v", err)
		}
		if waiting > 0 {
			return
		}
		if time.Now().After(deadline) {
			t.Fatal("no session ever waited on a lock: the press did not reach the granted row, " +
				"so this case interleaved nothing and would pass over the defect it is named for")
		}
	}
}

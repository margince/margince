// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// An approved effect takes the authorization hold before it locks the
// approval row, the order identity documents for every holder. An effect that
// meets a role change in progress waits at the hold, with the approval row
// still free. The two finish one after the other instead of deadlocking.

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// authorizationChangeInProgress holds the authorization lock exclusively, as a
// role change does while it commits, until release is called.
func authorizationChangeInProgress(t *testing.T, e *integration.Env) (release func()) {
	t.Helper()
	tx, err := e.Pool.Begin(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(t.Context(), `SELECT pg_advisory_xact_lock(hashtext('margince:authz')::bigint)`); err != nil {
		t.Fatalf("holding the authorization lock: %v", err)
	}
	return func() {
		if err := tx.Commit(context.Background()); err != nil {
			t.Errorf("releasing the authorization lock: %v", err)
		}
	}
}

// waitUntilSomeoneWaitsForTheAuthorizationLock returns once another backend
// waits on the authorization lock. It polls the lock table, a bounded number
// of reads, rather than sleeping.
func waitUntilSomeoneWaitsForTheAuthorizationLock(t *testing.T, e *integration.Env) {
	t.Helper()
	for range 10_000 {
		var waiting int
		if err := e.Pool.QueryRow(t.Context(), `SELECT count(*) FROM pg_locks
			WHERE locktype = 'advisory' AND NOT granted
			  AND classid::bigint = (hashtext('margince:authz')::bigint >> 32) & 4294967295
			  AND objid::bigint = hashtext('margince:authz')::bigint & 4294967295`).Scan(&waiting); err != nil {
			t.Fatal(err)
		}
		if waiting > 0 {
			return
		}
	}
	t.Fatal("the effect never waited on the authorization lock, so it did not take it before the approval row")
}

func TestAnEffectWaitsForARoleChangeBeforeItLocksTheApproval(t *testing.T) {
	e := integration.Setup(t)
	svc := rateSvc(t, e)
	approver := e.As(e.Rep1, []ids.UUID{e.Team1}, integration.AdminPerms)
	id := stageProposal(approver, t, svc, fxRateProposalKind, fxRateTargetType, e.WS,
		map[string]string{"from_currency": "USD", "rate": "0.9"}, "USD → EUR 0.9")

	release := authorizationChangeInProgress(t, e)
	decided := make(chan error, 1)
	go func() {
		_, err := svc.Decide(approver, id, true, nil)
		decided <- err
	}()
	waitUntilSomeoneWaitsForTheAuthorizationLock(t, e)

	// The effect waits at the hold, so the approval row is still free. Had it
	// locked the row first, a role change that then wrote that member's
	// approvals would close the cycle.
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		var locked ids.UUID
		return tx.QueryRow(t.Context(), `SELECT id FROM approval WHERE id = $1 FOR UPDATE NOWAIT`, id).Scan(&locked)
	}); err != nil {
		t.Fatalf("the approval row was locked while the effect waited for the authorization hold: %v", err)
	}

	release()
	if err := <-decided; err != nil {
		t.Fatalf("the effect did not complete once the role change committed: %v", err)
	}
	if n := ratesAt(t, e, "USD", 0.9); n != 1 {
		t.Errorf("%d USD rows at 0.9 after both finished, want 1", n)
	}
}

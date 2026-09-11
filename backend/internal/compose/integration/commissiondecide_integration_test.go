// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// Two decisions about one ledger entry take turns. The transition is decided
// from the entry's status, so that status has to be read under the row lock:
// read before it, both decisions see `paid`, both pass the lifecycle, and the
// ledger records the same clawback twice.

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/commissions"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestTwoConcurrentVoidsOfOnePaymentWriteOneReversal(t *testing.T) {
	e := Setup(t)
	fx := seedAccrualFixture(t, e, "tier2_20")
	admin := e.As(e.AdminUser, nil, commissionAdminPerms)

	winAndDeliver(t, e, fx)
	page, err := fx.ledger.List(admin, commissions.ListInput{DealID: &fx.deal})
	if err != nil || len(page.Data) != 1 {
		t.Fatalf("ledger after the win: %v %+v", err, page.Data)
	}
	entry := ids.From[ids.CommissionEntryKind](ids.UUID(page.Data[0].Id))
	for _, decision := range []string{commissions.DecisionApprove, commissions.DecisionPay} {
		if _, err := fx.ledger.Decide(admin, entry, commissions.DecideInput{Decision: decision}); err != nil {
			t.Fatalf("%s: %v", decision, err)
		}
	}

	// A third transaction holds the entry's row lock while both voids start, so
	// the two are certain to overlap rather than hoped to: each is released only
	// once both are waiting on the lock. A void that read the status before
	// asking for the lock has by then read `paid` exactly like its twin.
	ctx := context.Background()
	hold, err := OwnerConn(t).Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := hold.Exec(ctx, `SELECT 1 FROM commission_entry WHERE id = $1 FOR UPDATE`, entry); err != nil {
		t.Fatal(err)
	}
	var holder int32
	if err := hold.QueryRow(ctx, `SELECT pg_backend_pid()`).Scan(&holder); err != nil {
		t.Fatal(err)
	}

	// Neither void sends If-Match — the optional header is exactly what leaves
	// the row lock as the only guard.
	reason := "partner clawback"
	errs := make([]error, 2)
	var wg sync.WaitGroup
	for i := range errs {
		wg.Go(func() {
			_, errs[i] = fx.ledger.Decide(e.As(e.AdminUser, nil, commissionAdminPerms), entry,
				commissions.DecideInput{Decision: commissions.DecisionVoid, Reason: &reason})
		})
	}
	awaitLockWaiters(t, holder, 2)
	if err := hold.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	wg.Wait()

	var voided, refused int
	for _, err := range errs {
		var illegal *commissions.IllegalTransitionError
		switch {
		case err == nil:
			voided++
		case errors.As(err, &illegal):
			refused++
		default:
			t.Fatalf("a concurrent void failed for a reason other than the lifecycle: %v", err)
		}
	}
	if voided != 1 || refused != 1 {
		t.Fatalf("voided=%d refused=%d, want one void and one refusal of the entry it had already voided", voided, refused)
	}

	var reversals int
	if err := e.DB().Tx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx,
			`SELECT count(*) FROM commission_entry WHERE reversal_of = $1`, entry).Scan(&reversals)
	}); err != nil {
		t.Fatal(err)
	}
	if reversals != 1 {
		t.Errorf("the ledger holds %d reversals of one payment, want exactly one", reversals)
	}
}

// awaitLockWaiters returns once n sessions are queued behind the holder's lock —
// THAT backend's, so no unrelated session can stand in for a void. The queue is
// followed rather than read one level deep: a row lock queues its waiters, so
// the second void waits behind the first rather than on the holder directly.
// The bound only turns a void that never reached the lock into a failure that
// says so, instead of a test that hangs.
func awaitLockWaiters(t *testing.T, holder int32, n int) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	watcher := OwnerConn(t)
	for {
		// pg_stat_activity is materialized once per transaction and cached, so
		// each poll clears it first on this same connection, or a void that
		// reached the lock after the first read would never be seen.
		if _, err := watcher.Exec(ctx, `SELECT pg_stat_clear_snapshot()`); err != nil {
			t.Fatalf("clearing the stats snapshot before probing the lock queue: %v", err)
		}
		var waiting int
		if err := watcher.QueryRow(ctx, `
			WITH RECURSIVE queued AS (
				SELECT pid FROM pg_stat_activity WHERE $1 = ANY(pg_blocking_pids(pid))
				UNION
				SELECT a.pid FROM pg_stat_activity a
				  JOIN queued q ON q.pid = ANY(pg_blocking_pids(a.pid)))
			SELECT count(*) FROM queued`, holder).Scan(&waiting); err != nil {
			t.Fatalf("waiting for %d sessions to block on the entry's lock: %v", n, err)
		}
		if waiting >= n {
			return
		}
	}
}

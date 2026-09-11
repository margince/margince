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
	awaitLockWaiters(t, 2)
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

// awaitLockWaiters returns once n sessions are blocked on a lock over the
// commission ledger. The bound only turns a void that never reached the lock
// into a failure that says so, instead of a test that hangs.
func awaitLockWaiters(t *testing.T, n int) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	watcher := OwnerConn(t)
	for {
		var waiting int
		if err := watcher.QueryRow(ctx, `
			SELECT count(*) FROM pg_stat_activity
			 WHERE datname = current_database() AND wait_event_type = 'Lock'
			   AND query LIKE '%commission_entry%'`).Scan(&waiting); err != nil {
			t.Fatalf("waiting for %d sessions to block on the entry's lock: %v", n, err)
		}
		if waiting >= n {
			return
		}
	}
}

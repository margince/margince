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

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/commissions"
	"github.com/margince/margince/backend/internal/platform/testdb"
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
	decided := make(chan struct{})
	go func() {
		wg.Wait()
		close(decided)
	}()
	// The voids wait on the hold, so a failing probe rolls it back before waiting.
	t.Cleanup(func() {
		if err := hold.Rollback(ctx); err != nil && !errors.Is(err, pgx.ErrTxClosed) {
			t.Logf("rolling back the held entry: %v", err)
		}
		<-decided
	})
	testdb.AwaitQueuedBehind(t, OwnerConn(t), holder, 2, decided, "the two voids")
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

// An agent approves, pays and voids a real entry on its user's grants. The audit
// names the agent, and the reversal row does too.
func TestAnAgentDecidesARealCommissionAndEachRowNamesIt(t *testing.T) {
	e := Setup(t)
	fx := seedAccrualFixture(t, e, "tier2_20")
	admin := e.As(e.AdminUser, nil, commissionAdminPerms)
	winAndDeliver(t, e, fx)
	page, err := fx.ledger.List(admin, commissions.ListInput{DealID: &fx.deal})
	if err != nil || len(page.Data) != 1 {
		t.Fatalf("ledger after the win: %v %+v", err, page.Data)
	}
	entry := ids.From[ids.CommissionEntryKind](ids.UUID(page.Data[0].Id))
	passport := e.SeedPassportFor(t, OwnerConn(t), "commission agent", e.AdminUser)
	agent := e.AgentOnPassport(t, e.AdminUser, nil, commissionAdminPerms, ids.From[ids.PassportKind](passport))

	for _, decision := range []string{commissions.DecisionApprove, commissions.DecisionPay} {
		if _, err := fx.ledger.Decide(agent, entry, commissions.DecideInput{Decision: decision}); err != nil {
			t.Fatalf("the agent's %s: %v", decision, err)
		}
	}
	reason := "partner clawback"
	if _, err := fx.ledger.Decide(agent, entry, commissions.DecideInput{Decision: commissions.DecisionVoid, Reason: &reason}); err != nil {
		t.Fatalf("the agent's void: %v", err)
	}
	ctx := context.Background()
	var reversalBy string
	if err := OwnerConn(t).QueryRow(ctx, `SELECT captured_by FROM commission_entry WHERE reversal_of = $1`, entry).Scan(&reversalBy); err != nil {
		t.Fatalf("reading the reversal: %v", err)
	}
	want := "agent:" + passport.String()
	if reversalBy != want {
		t.Errorf("the reversal names %q, want %q", reversalBy, want)
	}
	var byAgent, byOthers int
	if err := OwnerConn(t).QueryRow(ctx, `
		SELECT count(*) FILTER (WHERE actor_id = $2), count(*) FILTER (WHERE action <> 'accrue' AND actor_id <> $2)
		  FROM audit_log WHERE entity_type = 'commission' AND entity_id = $1`, entry, want).Scan(&byAgent, &byOthers); err != nil {
		t.Fatalf("reading the audit rows: %v", err)
	}
	if byAgent != 3 || byOthers != 0 {
		t.Errorf("the audit rows name the agent %d times and someone else %d times, want 3 and 0", byAgent, byOthers)
	}
}

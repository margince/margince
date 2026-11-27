// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package contacts

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// A reader of the onboarding state and a writer of the anchor company take
// turns: SaveCompany waits on the advisory lock while a transaction that
// already read the state is open, and runs once that transaction ends.
func TestSaveCompanyWaitsForATransactionHoldingTheCompanyStateLock(t *testing.T) {
	e := newAnchorEnv(t)
	ctx := e.ctx
	locked, release := make(chan struct{}), make(chan struct{})
	held := make(chan error, 1)
	go func() {
		held <- e.store.tx(ctx, func(tx pgx.Tx) error {
			if _, _, err := e.store.OnboardingCompanyState(ctx, tx); err != nil {
				return err
			}
			close(locked)
			<-release
			return nil
		})
	}()
	select {
	case <-locked:
	case err := <-held:
		t.Fatalf("the reading transaction ended before it held the lock: %v", err)
	}
	saved := make(chan error, 1)
	go func() {
		_, err := e.store.SaveCompany(ctx, SaveCompanyInput{DisplayName: "Our Company Renamed"})
		saved <- err
	}()

	deadline := time.Now().Add(10 * time.Second)
	for !advisoryLockHasAWaiter(ctx, t, e.pool) {
		select {
		case err := <-saved:
			t.Fatalf("SaveCompany finished (%v) while another transaction held the company state lock", err)
		default:
		}
		if time.Now().After(deadline) {
			t.Fatal("SaveCompany never queued on the company state lock")
		}
	}
	close(release)
	if err := <-held; err != nil {
		t.Fatalf("the reading transaction: %v", err)
	}
	if err := <-saved; err != nil {
		t.Fatalf("SaveCompany after the lock was released: %v", err)
	}
}

func advisoryLockHasAWaiter(ctx context.Context, t *testing.T, pool *pgxpool.Pool) bool {
	t.Helper()
	var waiting bool
	if err := pool.QueryRow(ctx,
		`SELECT EXISTS (SELECT 1 FROM pg_locks WHERE locktype = 'advisory' AND NOT granted)`).Scan(&waiting); err != nil {
		t.Fatalf("reading the lock queue: %v", err)
	}
	return waiting
}

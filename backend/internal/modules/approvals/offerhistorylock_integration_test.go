// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package approvals

import (
	"context"
	"fmt"
	"testing"

	"github.com/jackc/pgx/v5"
)

func TestAnEmptyProposalHistorySerializesCompetingReadings(t *testing.T) {
	e := setupStaging(t)
	target := e.seedCompany(t)
	ctx, cancel := context.WithCancel(e.asAgent(t))
	defer cancel()
	first := e.competingTx(t)
	offers, err := e.svc.OffersForTx(ctx, first, "enrich", target)
	if err != nil || len(offers) != 0 {
		t.Fatalf("initial empty history: offers=%d err=%v", len(offers), err)
	}
	blocker := backendPID(t, first)
	result, done := make(chan error, 1), make(chan struct{})
	go func() {
		defer close(done)
		result <- e.svc.db.Tx(ctx, func(tx pgx.Tx) error {
			offers, err := e.svc.OffersForTx(ctx, tx, "enrich", target)
			if err == nil && len(offers) != 0 {
				return fmt.Errorf("empty history acquired %d offers", len(offers))
			}
			return err
		})
	}()
	defer func() { cancel(); <-done }()
	waitForRowLockWaiter(t, e, blocker, done)
	if err := first.Rollback(ctx); err != nil {
		t.Fatal(err)
	}
	if err := <-result; err != nil {
		t.Fatal(err)
	}
}

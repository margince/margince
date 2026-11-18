// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package deals

import (
	"testing"

	"github.com/jackc/pgx/v5"
)

// Two creates on one deal both lock the deal row twice: once to admit the
// offer, once to admit its lines. The second create must wait at the first
// lock, or each holds a shared lock the other needs and Postgres aborts one.
func TestTwoOfferCreatesOnOneDealBothLand(t *testing.T) {
	e := setupSeamEnv(t)
	deal := offerNumberDeal(t, e)
	input := CreateOfferInput{Currency: "EUR", Source: "manual"}
	store := e.provider.store
	ctx := e.passport()

	admitted := make(chan struct{})
	release := make(chan struct{})
	first := make(chan error, 1)
	go func() {
		first <- store.Tx(ctx, func(tx pgx.Tx) error {
			in, err := admitOfferCreation(ctx, tx, deal, input)
			if err != nil {
				return err
			}
			close(admitted)
			<-release
			_, _, _, err = admitOfferLines(ctx, tx, deal, in)
			return err
		})
	}()
	select {
	case <-admitted:
	case err := <-first:
		t.Fatalf("admitting the first create: %v", err)
	}

	second := make(chan error, 1)
	go func() {
		second <- store.Tx(ctx, func(tx pgx.Tx) error {
			in, err := admitOfferCreation(ctx, tx, deal, input)
			if err != nil {
				return err
			}
			_, _, _, err = admitOfferLines(ctx, tx, deal, in)
			return err
		})
	}()
	waitForALockWaiter(t, e.configEnv)
	close(release)

	if err := <-first; err != nil {
		t.Errorf("the first create: %v", err)
	}
	if err := <-second; err != nil {
		t.Errorf("the second create: %v", err)
	}
}

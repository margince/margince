// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"regexp"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/multitracer"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/testdb"
)

func TestAHoldReleaseRetriesWhenItsEvidenceMovesToAnotherComponent(t *testing.T) {
	e := integration.Setup(t)
	const oldKey, newKey, email = "hold-old-component", "hold-new-component", "held@customer.example"
	for _, input := range []activities.LogActivityInput{
		{Kind: "email", Source: "manual", ThreadKey: oldKey, CounterpartyEmail: email},
		{Kind: "email", Source: "manual", ThreadKey: newKey, CounterpartyEmail: "other@customer.example"},
	} {
		if _, _, err := e.Activities.LogActivity(e.Admin(), input); err != nil {
			t.Fatal(err)
		}
	}
	barrier := &threadSQLBarrier{match: regexp.MustCompile(`(?i)SELECT\s+DISTINCT\s+thread_key\s+FROM\s+activity`), entered: make(chan int, 1), release: make(chan struct{})}
	config := e.Pool.Config()
	config.ConnConfig.Tracer = multitracer.New(config.ConnConfig.Tracer, barrier)
	pool, err := testdb.OwnPoolFromConfig(t.Context(), config)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	finished, workerDone := make(chan error, 1), make(chan struct{})
	ctx, cancel := context.WithCancel(e.Admin())
	go func() {
		defer close(workerDone)
		finished <- database.WithWorkspaceTx(ctx, pool, func(tx pgx.Tx) error { return capture.LockHoldEvidenceTx(ctx, tx, []string{email}) })
	}()
	var once sync.Once
	release := func() { once.Do(func() { close(barrier.release) }) }
	t.Cleanup(func() { cancel(); release(); <-workerDone })
	select {
	case <-barrier.entered:
	case err := <-finished:
		t.Fatalf("release missed its evidence boundary: %v", err)
	}
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error { return mergeThreadsTx(e.Admin(), tx, oldKey, newKey) }); err != nil {
		t.Fatal(err)
	}
	release()
	if err := <-finished; !storekit.IsLockCycle(err) {
		t.Fatalf("changed evidence returned %v, want native transaction retry", err)
	}
	if err := withRetryingWorkspaceTx(e.Admin(), pool, func(tx pgx.Tx) error { return capture.LockHoldEvidenceTx(e.Admin(), tx, []string{email}) }); err != nil {
		t.Fatal(err)
	}
}

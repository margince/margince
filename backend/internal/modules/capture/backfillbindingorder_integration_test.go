// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package capture_test

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/testdb"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

type accountBoundaryBarrier struct {
	entered chan struct{}
	release chan struct{}
	once    sync.Once
}

func (b *accountBoundaryBarrier) TraceQueryStart(ctx context.Context, _ *pgx.Conn, query pgx.TraceQueryStartData) context.Context {
	if strings.Contains(query.SQL, "FROM capture_connection") {
		b.once.Do(func() {
			close(b.entered)
			select {
			case <-b.release:
			case <-ctx.Done():
			}
		})
	}
	return ctx
}

func (*accountBoundaryBarrier) TraceQueryEnd(context.Context, *pgx.Conn, pgx.TraceQueryEndData) {}

func TestHistoryAccountCohortsFollowConnectionLockOrder(t *testing.T) {
	for _, rebindStartsFirst := range []bool{false, true} {
		t.Run(map[bool]string{false: "import_transaction_starts_first", true: "rebind_transaction_starts_first"}[rebindStartsFirst], func(t *testing.T) {
			ctx, _, vault, workspace := newCaptureRegistryFixture(t)
			ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
			defer cancel()
			_, pool := setupCaptureDB(t)
			provider := &checkpointHistory{}
			registry := capture.NewRegistry(database.BindTo(pool, workspace), nil, fixtureAuthority{}, vault)
			registry.Register(provider)
			if _, err := registry.Connect(ctx, "gmail", connector.Auth("first")); err != nil {
				t.Fatal(err)
			}
			actor, ok := principal.Actor(ctx)
			if !ok {
				t.Fatal("fixture actor missing")
			}
			start := func(reg *capture.Registry) (capture.BackfillRun, error) {
				return reg.StartBackfill(ctx, "gmail", ids.From[ids.UserKind](actor.UserID), 3,
					connector.BackfillEstimate{Messages: 20}, func(context.Context, pgx.Tx, ids.UUID) error { return nil })
			}
			barrier := &accountBoundaryBarrier{entered: make(chan struct{}), release: make(chan struct{})}
			config := pool.Config()
			config.ConnConfig.Tracer = barrier
			delayedPool, err := testdb.OwnPoolFromConfig(ctx, config)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(delayedPool.Close)
			delayed := capture.NewRegistry(database.BindTo(delayedPool, workspace), nil, fixtureAuthority{}, vault)
			delayed.Register(provider)
			type bindingResult struct {
				run capture.BackfillRun
				err error
			}
			finished, done := make(chan bindingResult, 1), make(chan struct{})
			go func() {
				defer close(done)
				if rebindStartsFirst {
					_, err := delayed.Connect(ctx, "gmail", connector.Auth("second"))
					finished <- bindingResult{err: err}
					return
				}
				run, err := start(delayed)
				finished <- bindingResult{run: run, err: err}
			}()
			var releaseOnce sync.Once
			unblock := func() { releaseOnce.Do(func() { close(barrier.release) }) }
			t.Cleanup(func() { cancel(); unblock(); <-done })
			select {
			case <-barrier.entered:
			case result := <-finished:
				t.Fatalf("transaction missed the connection barrier: %v", result.err)
			case <-ctx.Done():
				t.Fatal(ctx.Err())
			}
			var run capture.BackfillRun
			if rebindStartsFirst {
				run, err = start(registry)
			} else {
				_, err = registry.Connect(ctx, "gmail", connector.Auth("second"))
			}
			if err != nil {
				t.Fatal(err)
			}
			unblock()
			result := <-finished
			if result.err != nil {
				t.Fatal(result.err)
			}
			if !rebindStartsFirst {
				run = result.run
			}
			_, completed, _, err := registry.RunBackfillStep(ctx, run.ID)
			if rebindStartsFirst {
				if err != nil || completed || provider.calls != 0 {
					t.Fatalf("later rebind retained old account: calls=%d completed=%v err=%v", provider.calls, completed, err)
				}
			} else if !errors.Is(err, connector.ErrUnreachable) || completed || provider.calls != 1 {
				t.Fatalf("new account's import was refused: calls=%d completed=%v err=%v", provider.calls, completed, err)
			}
		})
	}
}

func TestConcurrentFirstGrantsStillRecordAnAccountChange(t *testing.T) {
	ctx, _, vault, workspace := newCaptureRegistryFixture(t)
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	owner, pool := setupCaptureDB(t)
	gate, err := owner.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var releaseOnce sync.Once
	unblock := func() {
		releaseOnce.Do(func() {
			if err := gate.Commit(context.Background()); err != nil {
				t.Error(err)
			}
		})
	}
	t.Cleanup(unblock)
	if _, err := gate.Exec(ctx, `LOCK TABLE capture_connection IN SHARE MODE`); err != nil {
		t.Fatal(err)
	}
	application := "capture-first-grants-" + ids.NewV7().String()
	config := pool.Config()
	config.ConnConfig.RuntimeParams["application_name"] = application
	grantPool, err := testdb.OwnPoolFromConfig(ctx, config)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(grantPool.Close)
	registry := capture.NewRegistry(database.BindTo(grantPool, workspace), nil, fixtureAuthority{}, vault)
	registry.Register(&checkpointHistory{})
	type grantResult struct {
		id  ids.UUID
		err error
	}
	results := make(chan grantResult, 2)
	var finished sync.WaitGroup
	for _, account := range []string{"first", "second"} {
		finished.Go(func() {
			id, err := registry.Connect(ctx, "gmail", connector.Auth(account))
			results <- grantResult{id, err}
		})
	}
	t.Cleanup(func() { cancel(); unblock(); finished.Wait() })
	for {
		if _, err := gate.Exec(ctx, `SELECT pg_stat_clear_snapshot()`); err != nil {
			t.Fatal(err)
		}
		var blocked int
		if err := gate.QueryRow(ctx, `SELECT count(*) FROM pg_stat_activity
 WHERE application_name=@application AND cardinality(pg_blocking_pids(pid))>0`, pgx.NamedArgs{"application": application}).Scan(&blocked); err != nil {
			t.Fatal(err)
		}
		if blocked == 2 {
			break
		}
		select {
		case result := <-results:
			t.Fatalf("grant bypassed the insert gate: %v", result.err)
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		default:
		}
	}
	unblock()
	first, second := <-results, <-results
	if first.err != nil || second.err != nil || first.id != second.id {
		t.Fatalf("concurrent grants did not share one connection: first=%v second=%v", first, second)
	}
	var generation int
	var bound bool
	if err := owner.QueryRow(ctx, `SELECT generation,account_bound_at IS NOT NULL FROM capture_connection WHERE id=@connection`, pgx.NamedArgs{"connection": first.id}).Scan(&generation, &bound); err != nil {
		t.Fatal(err)
	}
	if generation != 1 || !bound {
		t.Fatalf("a replacement account was recorded as an initial grant: generation=%d bound=%v", generation, bound)
	}
}

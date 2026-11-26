// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package database

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func readAdmissionPool(t *testing.T, connections int32) *pgxpool.Pool {
	t.Helper()
	cfg, err := PoolConfig(ceilingTestDSN(t))
	if err != nil {
		t.Fatal(err)
	}
	cfg.MaxConns, cfg.MinConns = connections, 0
	pool, err := openPool(t.Context(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	return pool
}

func TestSnapshotsAndBoundedReadsShareCapacityBeforeBorrowing(t *testing.T) {
	pool := readAdmissionPool(t, 4)
	workspace := ids.NewV7()
	ctx := principal.WithWorkspaceID(context.Background(), workspace)
	entered := make(chan struct{}, 2)
	release := make(chan struct{})
	finished := make(chan error, 2)
	for range 2 {
		go func() {
			finished <- WithReadAdmission(ctx, pool, func(ctx context.Context) error {
				return WithWorkspaceSnapshot(ctx, pool, func(ctx context.Context) error {
					return WithWorkspaceTx(ctx, pool, func(tx pgx.Tx) error {
						if _, err := tx.Exec(ctx, "SELECT 1"); err != nil {
							return err
						}
						entered <- struct{}{}
						<-release
						return nil
					})
				})
			})
		}()
	}
	finishedSeen := 0
	t.Cleanup(func() {
		close(release)
		for ; finishedSeen < 2; finishedSeen++ {
			if err := <-finished; err != nil {
				t.Error(err)
			}
		}
	})
	for holding := 0; holding < 2; {
		select {
		case <-entered:
			holding++
		case err := <-finished:
			finishedSeen++
			t.Fatalf("a snapshot worker ended before holding its connection: %v", err)
		case <-time.After(30 * time.Second):
			t.Fatal("two snapshot workers did not start holding connections within 30s")
		}
	}
	admission := admissionOf(pool)
	if admission.slots.TryAcquire(1) {
		admission.slots.Release(1)
		t.Fatal("two snapshots did not fill the heavy-read allowance on a four-connection pool")
	}
	if err := WithWorkspaceTx(ctx, pool, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, "SELECT 1")
		return err
	}); err != nil {
		t.Fatalf("an ordinary transaction could not use the reserved connections: %v", err)
	}
	ordinary, stop := context.WithTimeout(ctx, time.Second)
	defer stop()
	if err := WithWorkspaceSnapshot(ordinary, pool, func(context.Context) error { return nil }); err != nil {
		t.Fatalf("ordinary snapshot took heavy-read capacity: %v", err)
	}
	if err := BindTo(pool, ids.From[ids.WorkspaceKind](workspace)).Bounded(time.Second).Tx(ordinary, func(pgx.Tx) error { return nil }); err != nil {
		t.Fatalf("statement budget opted an ordinary transaction into heavy reads: %v", err)
	}
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	metered, meter := WithStatementMeter(canceled)
	db := BindTo(pool, ids.From[ids.WorkspaceKind](workspace)).Bounded(time.Second).WithReadAdmission()
	err := db.Read(metered, func(context.Context, pgx.Tx) error {
		t.Error("a canceled read reached its transaction body")
		return nil
	})
	if !errors.Is(err, context.Canceled) || meter.Cost().Acquires != 0 {
		t.Fatalf("a bounded read queued behind snapshots = %v, cost=%+v; want cancellation before pool checkout", err, meter.Cost())
	}
}

func TestReadAdmissionReleasesFailedReadsAndHonorsTheFinalPoolSize(t *testing.T) {
	for _, tc := range []struct {
		connections int32
		capacity    int64
	}{{1, 1}, {2, 1}, {4, 2}, {16, 3}} {
		t.Run(fmt.Sprint(tc.connections), func(t *testing.T) {
			pool := readAdmissionPool(t, tc.connections)
			refused := errors.New("read failed")
			err := WithReadAdmission(t.Context(), pool, func(ctx context.Context) error {
				return WithReadAdmission(ctx, pool, func(context.Context) error { return refused })
			})
			if !errors.Is(err, refused) {
				t.Fatalf("read returned %v, want its own failure", err)
			}
			admission := admissionOf(pool)
			if !admission.slots.TryAcquire(tc.capacity) {
				t.Fatal("the failed or nested read leaked capacity")
			}
			defer admission.slots.Release(tc.capacity)
			if admission.slots.TryAcquire(1) {
				admission.slots.Release(1)
				t.Fatalf("the pool admitted more than %d heavy reads", tc.capacity)
			}
		})
	}
}

func TestAnExpiredReadContextMustReacquireCapacity(t *testing.T) {
	pool := readAdmissionPool(t, 2)
	var saved context.Context
	if err := WithReadAdmission(t.Context(), pool, func(ctx context.Context) error {
		saved = ctx
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	admission := admissionOf(pool)
	if !admission.slots.TryAcquire(1) {
		t.Fatal("the completed read leaked its allowance")
	}
	defer admission.slots.Release(1)
	canceled, cancel := context.WithCancel(saved)
	cancel()
	called := false
	err := WithReadAdmission(canceled, pool, func(context.Context) error {
		called = true
		return nil
	})
	if called || !errors.Is(err, context.Canceled) {
		t.Fatalf("an expired permit bypassed admission: called=%v, error=%v", called, err)
	}
}

func TestAdmissionIsIndependentOfTracersAndCopiedPoolConfigurations(t *testing.T) {
	cfg, err := PoolConfig(ceilingTestDSN(t))
	if err != nil {
		t.Fatal(err)
	}
	cfg.MaxConns, cfg.MinConns = 2, 0
	cfg.ConnConfig.Tracer = nil
	small, err := openPool(t.Context(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer small.Close()
	copied := cfg.Copy()
	copied.MaxConns = 16
	large, err := openPool(t.Context(), copied)
	if err != nil {
		t.Fatal(err)
	}
	defer large.Close()
	if admissionOf(small) == admissionOf(large) {
		t.Fatal("independent pools share admission")
	}
	if !admissionOf(large).slots.TryAcquire(DefaultReadConcurrency) {
		t.Fatal("copied config retained the smaller pool's capacity")
	}
	defer admissionOf(large).slots.Release(DefaultReadConcurrency)
	db := BindTo(small, ids.From[ids.WorkspaceKind](ids.NewV7())).WithReadAdmission().Bounded(time.Second)
	if err := db.Read(t.Context(), func(ctx context.Context, _ pgx.Tx) error {
		return WithReadAdmission(ctx, small, func(context.Context) error { return nil })
	}); err != nil {
		t.Fatalf("nested read or custom tracer failed: %v", err)
	}
}

func TestReadAdmissionUsesTheConfiguredLimitAndRefusesLiveChanges(t *testing.T) {
	for _, limit := range []int{1, 3, 5, 100} {
		t.Run(fmt.Sprint(limit), func(t *testing.T) {
			pool := readAdmissionPool(t, 16)
			if err := ConfigureReadAdmission(pool, limit); err != nil {
				t.Fatal(err)
			}
			admission := admissionOf(pool)
			capacity := int64(min(limit, 8))
			if !admission.slots.TryAcquire(capacity) {
				t.Fatal("configured capacity is unavailable")
			}
			defer admission.slots.Release(capacity)
			if admission.slots.TryAcquire(1) {
				admission.slots.Release(1)
				t.Fatal("admitted beyond the configured or pool limit")
			}
			if err := ConfigureReadAdmission(pool, limit+1); err == nil {
				t.Fatal("replaced live admission")
			}
		})
	}
	pool := readAdmissionPool(t, 16)
	if err := ConfigureReadAdmission(pool, 0); err == nil {
		t.Fatal("accepted a zero read allowance")
	}
}

func TestReadAdmissionCannotReplaceALazyAllowanceWithAHeldPermit(t *testing.T) {
	pool := readAdmissionPool(t, 16)
	admission := admissionOf(pool)
	if err := WithReadAdmission(t.Context(), pool, func(context.Context) error {
		if err := ConfigureReadAdmission(pool, 1); err == nil {
			t.Fatal("replaced an allowance with a held permit")
		}
		if admissionOf(pool) != admission {
			t.Fatal("changed the permit's owner")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if !admission.slots.TryAcquire(DefaultReadConcurrency) {
		t.Fatal("the held permit did not return to its original allowance")
	}
	admission.slots.Release(DefaultReadConcurrency)
}

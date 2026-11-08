// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package capture_test

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

type recordedBackfillQueue struct {
	mu          sync.Mutex
	generations map[ids.UUID]int64
	refused     error
}

func (q *recordedBackfillQueue) scheduler() capture.BackfillScheduler {
	return capture.BackfillScheduler{
		Enqueue: func(_ context.Context, _ pgx.Tx, id ids.UUID, generation int64, _ time.Time) error {
			q.mu.Lock()
			defer q.mu.Unlock()
			if q.refused != nil {
				return q.refused
			}
			q.generations[id] = generation
			return nil
		},
		Executing: func(context.Context, pgx.Tx, ids.UUID, int64) (bool, error) { return false, nil },
	}
}

func admissionRun(t *testing.T) (context.Context, *capture.Registry, ids.UUID) {
	t.Helper()
	return admissionRunWith(t, fixtureConnector{})
}

func admissionRunWith(t *testing.T, provider connector.Connector) (context.Context, *capture.Registry, ids.UUID) {
	t.Helper()
	ctx, _, vault, ws := newCaptureRegistryFixture(t)
	_, pool := setupCaptureDB(t)
	reg := capture.NewRegistry(database.BindTo(pool, ws), nil, fixtureAuthority{}, vault)
	reg.Register(provider)
	if _, err := reg.Connect(ctx, "gmail", connector.Auth("fixture-token")); err != nil {
		t.Fatal(err)
	}
	actor, ok := principal.Actor(ctx)
	if !ok {
		t.Fatal("actor not bound")
	}
	run, err := reg.StartBackfill(ctx, "gmail", ids.From[ids.UserKind](actor.UserID), 3, connector.BackfillEstimate{Messages: 100},
		func(context.Context, pgx.Tx, ids.UUID) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	return ctx, reg, run.ID
}

func TestIndependentRegistriesShareOneInstallationImportBudget(t *testing.T) {
	for _, limit := range []int{1, 2} {
		t.Run(fmt.Sprintf("limit_%d", limit), func(t *testing.T) {
			ctx, first, firstID := admissionRun(t)
			_, second, secondID := admissionRun(t)
			_, third, _ := admissionRun(t)
			first.WithBackfillAdmission(limit, false)
			second.WithBackfillAdmission(limit, false)
			third.WithBackfillAdmission(limit, false)
			queue := &recordedBackfillQueue{generations: map[ids.UUID]int64{}}
			start := make(chan struct{})
			failures := make(chan error, 3)
			for _, registry := range []*capture.Registry{first, second, third} {
				go func() { <-start; failures <- registry.DispatchBackfills(ctx, queue.scheduler(), false) }()
			}
			close(start)
			for range 3 {
				if err := <-failures; err != nil {
					t.Fatal(err)
				}
			}
			if len(queue.generations) != limit {
				t.Fatalf("admitted %d imports, limit %d", len(queue.generations), limit)
			}
			owner, _ := setupCaptureDB(t)
			var running, queued int
			if err := owner.QueryRow(ctx, `SELECT count(*) FILTER(WHERE status='running'),count(*) FILTER(WHERE status='queued') FROM capture_backfill`).Scan(&running, &queued); err != nil {
				t.Fatal(err)
			}
			if running != limit || queued != 3-limit {
				t.Fatalf("running=%d queued=%d", running, queued)
			}
			for id, generation := range queue.generations {
				if generation == 0 {
					t.Fatal("admission enqueued legacy generation zero")
				}
				var stored int64
				args := []any{id}
				if err := owner.QueryRow(ctx, fmt.Sprintf("SELECT execution_generation FROM capture_backfill WHERE id=$%d", len(args)), args...).Scan(&stored); err != nil {
					t.Fatal(err)
				}
				if stored != generation {
					t.Fatalf("queued generation=%d stored=%d", generation, stored)
				}
				registry := first
				if id == secondID {
					registry = second
				}
				if id != firstID && id != secondID {
					registry = third
				}
				done, _, _, err := registry.RunBackfillExecution(ctx, id, generation, queue.scheduler())
				if !done || !errors.Is(err, capture.ErrBackfillUnsupported) {
					t.Fatalf("terminal step: done=%v err=%v", done, err)
				}
				break
			}
			if err := owner.QueryRow(ctx, `SELECT count(*) FROM capture_backfill WHERE status='running'`).Scan(&running); err != nil {
				t.Fatal(err)
			}
			if running != limit {
				t.Fatalf("terminal run did not hand its slot to the queue: %d", running)
			}
		})
	}
}

func TestARefusedAdmissionEnqueueRollsBackThePromotion(t *testing.T) {
	ctx, reg, id := admissionRun(t)
	refusal := errors.New("queue unavailable")
	queue := &recordedBackfillQueue{generations: map[ids.UUID]int64{}, refused: refusal}
	if err := reg.DispatchBackfills(ctx, queue.scheduler(), false); !errors.Is(err, refusal) {
		t.Fatalf("dispatch: %v", err)
	}
	owner, _ := setupCaptureDB(t)
	args := []any{id}
	var status string
	var generation int64
	if err := owner.QueryRow(ctx, fmt.Sprintf(`SELECT status,execution_generation FROM capture_backfill WHERE id=$%d`, len(args)), args...).Scan(&status, &generation); err != nil {
		t.Fatal(err)
	}
	if status != "queued" || generation != 0 {
		t.Fatalf("promotion survived refused enqueue: %s %d", status, generation)
	}
}

type rateLimitedHistory struct {
	fixtureConnector
	calls atomic.Int64
	name  string
}

func (p *rateLimitedHistory) Descriptor() connector.Descriptor {
	name := p.name
	if name == "" {
		name = "graph"
	}
	return connector.Descriptor{Name: name, Version: "fixture"}
}

func (p *rateLimitedHistory) EstimateBackfill(context.Context, connector.Auth, time.Time) (connector.BackfillEstimate, error) {
	return connector.BackfillEstimate{Messages: 100}, nil
}

func (p *rateLimitedHistory) BackfillPage(context.Context, connector.Auth, time.Time, string, connector.Sink) (connector.BackfillPageResult, error) {
	p.calls.Add(1)
	return connector.BackfillPageResult{}, &connector.RateLimitedError{RetryAfter: time.Hour}
}

func TestHoldAndLowerLimitPreserveTheQueuedRunsRetryDeadline(t *testing.T) {
	ctx, registry, firstID := admissionRun(t)
	registry.WithBackfillAdmission(2, false)
	provider := &rateLimitedHistory{}
	registry.Register(provider)
	if _, err := registry.Connect(ctx, "graph", connector.Auth("fixture-token")); err != nil {
		t.Fatal(err)
	}
	actor, ok := principal.Actor(ctx)
	if !ok {
		t.Fatal("actor not bound")
	}
	second, err := registry.StartBackfill(ctx, "graph", ids.From[ids.UserKind](actor.UserID), 3, connector.BackfillEstimate{Messages: 100},
		func(context.Context, pgx.Tx, ids.UUID) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	queue := &recordedBackfillQueue{generations: map[ids.UUID]int64{}}
	if err := registry.DispatchBackfills(ctx, queue.scheduler(), false); err != nil {
		t.Fatal(err)
	}
	generation := queue.generations[second.ID]
	done, _, _, err := registry.RunBackfillExecution(ctx, second.ID, generation, queue.scheduler())
	if !done || !errors.Is(err, connector.ErrRateLimited) {
		t.Fatalf("provider backoff: %v %v", done, err)
	}
	generation = queue.generations[second.ID]
	owner, _ := setupCaptureDB(t)
	var due time.Time
	args := []any{second.ID}
	if err := owner.QueryRow(ctx, fmt.Sprintf("SELECT execution_due_at FROM capture_backfill WHERE id=$%d", len(args)), args...).Scan(&due); err != nil {
		t.Fatal(err)
	}
	registry.WithBackfillAdmission(1, true)
	if err := registry.DispatchBackfills(ctx, queue.scheduler(), true); err != nil {
		t.Fatal(err)
	}
	done, _, wait, err := registry.RunBackfillExecution(ctx, second.ID, generation, queue.scheduler())
	if done || wait <= 0 || err != nil || provider.calls.Load() != 1 {
		t.Fatalf("held page: done=%v wait=%v err=%v calls=%d", done, wait, err, provider.calls.Load())
	}
	registry.WithBackfillAdmission(1, false)
	if err := registry.DispatchBackfills(ctx, queue.scheduler(), true); err != nil {
		t.Fatal(err)
	}
	var status string
	var storedGeneration int64
	var storedDue time.Time
	if err := owner.QueryRow(ctx, fmt.Sprintf("SELECT status,execution_generation,execution_due_at FROM capture_backfill WHERE id=$%d", len(args)), args...).Scan(&status, &storedGeneration, &storedDue); err != nil {
		t.Fatal(err)
	}
	if status != "queued" || storedGeneration <= generation || storedDue.Before(due) {
		t.Fatalf("excess run: %s gen=%d due=%v, wanted queued after %d and no earlier than %v", status, storedGeneration, storedDue, generation, due)
	}
	done, _, _, err = registry.RunBackfillExecution(ctx, second.ID, generation, queue.scheduler())
	if !done || err != nil || provider.calls.Load() != 1 {
		t.Fatalf("old excess job fetched: done=%v err=%v calls=%d", done, err, provider.calls.Load())
	}
	done, _, _, err = registry.RunBackfillExecution(ctx, firstID, queue.generations[firstID], queue.scheduler())
	if !done || !errors.Is(err, capture.ErrBackfillUnsupported) {
		t.Fatalf("first run: %v %v", done, err)
	}
	if err := owner.QueryRow(ctx, fmt.Sprintf("SELECT status,execution_generation,execution_due_at FROM capture_backfill WHERE id=$%d", len(args)), args...).Scan(&status, &storedGeneration, &storedDue); err != nil {
		t.Fatal(err)
	}
	if status != "queued" || storedGeneration <= generation || storedDue.Before(due) {
		t.Fatalf("waiting run lost its deadline: %s %d %v", status, storedGeneration, storedDue)
	}
	args = []any{second.ID}
	if _, err := owner.Exec(ctx, fmt.Sprintf(`UPDATE capture_backfill SET execution_due_at=now()-interval '1 second' WHERE id=$%d`, len(args)), args...); err != nil {
		t.Fatal(err)
	}
	if err := registry.DispatchBackfills(ctx, queue.scheduler(), true); err != nil {
		t.Fatal(err)
	}
	args = []any{second.ID}
	if err := owner.QueryRow(ctx, fmt.Sprintf(`SELECT status FROM capture_backfill WHERE id=$%d`, len(args)), args...).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != "running" {
		t.Fatalf("ready queued run was not admitted: %s", status)
	}
}

func TestOneRefusedAdmissionDoesNotBlockTheNextImport(t *testing.T) {
	ctx, reg, firstID := admissionRun(t)
	_, _, secondID := admissionRun(t)
	owner, _ := setupCaptureDB(t)
	args := []any{firstID}
	if _, err := owner.Exec(ctx, fmt.Sprintf(`UPDATE capture_backfill SET created_at=now()-interval '2 hours' WHERE id=$%d`, len(args)), args...); err != nil {
		t.Fatal(err)
	}
	queue := &recordedBackfillQueue{generations: map[ids.UUID]int64{}}
	scheduler := queue.scheduler()
	accepted := scheduler.Enqueue
	scheduler.Enqueue = func(ctx context.Context, tx pgx.Tx, id ids.UUID, generation int64, due time.Time) error {
		if id == firstID {
			_, err := tx.Exec(ctx, `SELECT 1/0`)
			return err
		}
		return accepted(ctx, tx, id, generation, due)
	}
	if err := reg.DispatchBackfills(ctx, scheduler, false); err == nil {
		t.Fatal("refused enqueue was not reported")
	}
	var status string
	args = []any{firstID}
	if err := owner.QueryRow(ctx, fmt.Sprintf(`SELECT status FROM capture_backfill WHERE id=$%d`, len(args)), args...).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != "queued" {
		t.Fatalf("refused import status=%s", status)
	}
	args = []any{secondID}
	if err := owner.QueryRow(ctx, fmt.Sprintf(`SELECT status FROM capture_backfill WHERE id=$%d`, len(args)), args...).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != "running" || queue.generations[secondID] == 0 {
		t.Fatalf("next import status=%s generation=%d", status, queue.generations[secondID])
	}
}

func TestMailboxBackoffYieldsItsSlotButSharedBackoffKeepsIt(t *testing.T) {
	for _, mailbox := range []bool{true, false} {
		t.Run(fmt.Sprint(mailbox), func(t *testing.T) {
			ctx, reg, firstID := admissionRun(t)
			queue := &recordedBackfillQueue{generations: map[ids.UUID]int64{}}
			if err := reg.DispatchBackfills(ctx, queue.scheduler(), false); err != nil {
				t.Fatal(err)
			}
			_, _, secondID := admissionRun(t)
			owner, _ := setupCaptureDB(t)
			args := []any{firstID, mailbox}
			if _, err := owner.Exec(ctx, fmt.Sprintf(`UPDATE capture_backfill SET execution_due_at=now()+interval '1 hour', backoff_is_mailbox_scoped=$%d WHERE id=$%d`, len(args), len(args)-1), args...); err != nil {
				t.Fatal(err)
			}
			if err := reg.DispatchBackfills(ctx, queue.scheduler(), false); err != nil {
				t.Fatal(err)
			}
			var status string
			args = []any{secondID}
			if err := owner.QueryRow(ctx, fmt.Sprintf(`SELECT status FROM capture_backfill WHERE id=$%d`, len(args)), args...).Scan(&status); err != nil {
				t.Fatal(err)
			}
			wanted := "queued"
			if mailbox {
				wanted = "running"
			}
			if status != wanted {
				t.Fatalf("other import=%s want %s", status, wanted)
			}
			var remaining time.Duration
			args = []any{firstID}
			if err := owner.QueryRow(ctx, fmt.Sprintf(`SELECT extract(epoch FROM execution_due_at-now())::bigint FROM capture_backfill WHERE id=$%d`, len(args)), args...).Scan(&remaining); err != nil {
				t.Fatal(err)
			}
			if remaining < 3500 {
				t.Fatalf("retry deadline lost: %d seconds", remaining)
			}
		})
	}
}

func TestARefusedSuccessorLeavesTheCurrentExecutionRetryable(t *testing.T) {
	ctx, reg, id := admissionRunWith(t, &rateLimitedHistory{name: "gmail"})
	queue := &recordedBackfillQueue{generations: map[ids.UUID]int64{}}
	if err := reg.DispatchBackfills(ctx, queue.scheduler(), false); err != nil {
		t.Fatal(err)
	}
	generation := queue.generations[id]
	refusal := errors.New("queue unavailable")
	queue.refused = refusal
	done, completed, wait, err := reg.RunBackfillExecution(ctx, id, generation, queue.scheduler())
	if done || completed || wait < time.Hour || !errors.Is(err, refusal) {
		t.Fatalf("failed successor: done=%v complete=%v wait=%v err=%v", done, completed, wait, err)
	}
	owner, _ := setupCaptureDB(t)
	var status string
	var stored int64
	var failures int
	args := []any{id}
	if err := owner.QueryRow(ctx, fmt.Sprintf(`SELECT status,execution_generation,consecutive_failures FROM capture_backfill WHERE id=$%d`, len(args)), args...).Scan(&status, &stored, &failures); err != nil {
		t.Fatal(err)
	}
	if status != "running" || stored != generation || failures != 0 {
		t.Fatalf("successor rollback: status=%s generation=%d failures=%d", status, stored, failures)
	}
}

type blockedHistory struct {
	rateLimitedHistory
	entered chan struct{}
	release chan struct{}
}

func (p *blockedHistory) BackfillPage(ctx context.Context, _ connector.Auth, _ time.Time, _ string, _ connector.Sink) (connector.BackfillPageResult, error) {
	close(p.entered)
	select {
	case <-p.release:
		return connector.BackfillPageResult{NextToken: "next"}, nil
	case <-ctx.Done():
		return connector.BackfillPageResult{}, ctx.Err()
	}
}

func TestLoweringAdmissionWaitsForTheActivePagesLocks(t *testing.T) {
	ctx, first, _ := admissionRun(t)
	provider := &blockedHistory{name: "gmail", entered: make(chan struct{}), release: make(chan struct{})}
	otherCtx, second, id := admissionRunWith(t, provider)
	first.WithBackfillAdmission(2, false)
	second.WithBackfillAdmission(2, false)
	queue := &recordedBackfillQueue{generations: map[ids.UUID]int64{}}
	if err := first.DispatchBackfills(ctx, queue.scheduler(), false); err != nil {
		t.Fatal(err)
	}
	stopped := make(chan error, 1)
	pageCtx, cancel := context.WithCancel(otherCtx)
	defer cancel()
	var release sync.Once
	unblock := func() { release.Do(func() { close(provider.release) }) }
	finished := make(chan struct{})
	t.Cleanup(func() { cancel(); unblock(); <-finished })
	go func() {
		defer close(finished)
		_, _, _, err := second.RunBackfillExecution(pageCtx, id, queue.generations[id], queue.scheduler())
		stopped <- err
	}()
	select {
	case <-provider.entered:
	case err := <-stopped:
		t.Fatalf("provider did not start: %v", err)
	}
	second.WithBackfillAdmission(1, false)
	if err := second.DispatchBackfills(otherCtx, queue.scheduler(), true); err != nil {
		t.Fatal(err)
	}
	owner, _ := setupCaptureDB(t)
	var status string
	args := []any{id}
	if err := owner.QueryRow(ctx, fmt.Sprintf(`SELECT status FROM capture_backfill WHERE id=$%d`, len(args)), args...).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != "running" {
		t.Fatalf("active page was requeued before releasing its locks: %s", status)
	}
	unblock()
	if err := <-stopped; err != nil {
		t.Fatal(err)
	}
	args = []any{id}
	if err := owner.QueryRow(ctx, fmt.Sprintf(`SELECT status FROM capture_backfill WHERE id=$%d`, len(args)), args...).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != "queued" {
		t.Fatalf("idle excess page did not return to the queue: %s", status)
	}
}

func TestAnEnrichmentReservationIsRefundedAfterItsPageLosesOwnership(t *testing.T) {
	ctx, _, _, ws := newCaptureRegistryFixture(t)
	_, pool := setupCaptureDB(t)
	store := capture.NewAutoEnrichStore(database.BindTo(pool, ws))
	slot, err := store.ReserveBudget(ctx, 1)
	if err != nil || !slot.Reserved {
		t.Fatalf("initial reservation: %+v %v", slot, err)
	}
	stale := database.WithTransactionGuard(ctx, func(context.Context, pgx.Tx) error { return apperrors.ErrConflict })
	if _, err := store.ReserveBudget(stale, 1); !errors.Is(err, apperrors.ErrConflict) {
		t.Fatalf("stale page could reserve more budget: %v", err)
	}
	if err := store.ReleaseBudget(context.WithoutCancel(stale), slot); err != nil {
		t.Fatal(err)
	}
	next, err := store.ReserveBudget(ctx, 1)
	if err != nil || !next.Reserved {
		t.Fatalf("lost page leaked its reservation: %+v %v", next, err)
	}
}

// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package capture

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/multitracer"

	"github.com/margince/margince/backend/internal/compose"
	capturemod "github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/platform/testdb"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

type importThreadBarrier struct {
	armed            atomic.Bool
	once             sync.Once
	entered, release chan struct{}
	started          chan struct{}
	startOnce        sync.Once
}

type importThreadBarrierKey struct{}

func (b *importThreadBarrier) TraceQueryStart(ctx context.Context, _ *pgx.Conn, q pgx.TraceQueryStartData) context.Context {
	if b.armed.Load() && strings.Contains(q.SQL, "pg_advisory_xact_lock(key)") {
		if b.started != nil {
			b.startOnce.Do(func() { close(b.started) })
		}
		return context.WithValue(ctx, importThreadBarrierKey{}, true)
	}
	return ctx
}

func (b *importThreadBarrier) TraceQueryEnd(ctx context.Context, _ *pgx.Conn, q pgx.TraceQueryEndData) {
	locked, ok := ctx.Value(importThreadBarrierKey{}).(bool)
	if !ok || !locked || q.Err != nil {
		return
	}
	b.once.Do(func() {
		close(b.entered)
		select {
		case <-b.release:
		case <-ctx.Done():
		}
	})
}

type independentImportPage struct {
	*mailBatchConnector
	barrier *importThreadBarrier
}

func (p *independentImportPage) EstimateBackfill(context.Context, connector.Auth, time.Time) (connector.BackfillEstimate, error) {
	return connector.BackfillEstimate{Messages: len(p.raws)}, nil
}

func (p *independentImportPage) BackfillPage(ctx context.Context, auth connector.Auth, _ time.Time, _ string, sink connector.Sink) (connector.BackfillPageResult, error) {
	if p.barrier != nil {
		p.barrier.armed.Store(true)
	}
	_, err := p.Sync(ctx, auth, nil, sink)
	return connector.BackfillPageResult{Scanned: len(p.raws), Captured: len(p.raws)}, err
}

func TestTwoAdmittedImportsOnIndependentThreadsDoNotWaitForEachOther(t *testing.T) {
	independentImports(t, concurrentImportCase{})
}

func TestTwoAdmittedImportsCanMergeIndependentComponentsConcurrently(t *testing.T) {
	independentImports(t, concurrentImportCase{merge: true})
}

func TestOverlappingHistoryImportsReReadTheirCanonicalComponents(t *testing.T) {
	independentImports(t, concurrentImportCase{merge: true, overlap: true})
}

func TestConcurrentImportsFindAParentAfterItsUncommittedReply(t *testing.T) {
	independentImports(t, concurrentImportCase{forward: true, overlap: true})
}

func TestAFullReferenceHeaderDoesNotTakeTheInstallationBarrier(t *testing.T) {
	independentImports(t, concurrentImportCase{manyReferences: true})
}

type (
	concurrentImportCase struct{ merge, overlap, forward, manyReferences bool }
	concurrentMail       struct {
		first, second     []byte
		firstID, secondID string
		messages          []string
	}
)

func concurrentMailFixture(t *testing.T, env captureEnv, scenario concurrentImportCase) concurrentMail {
	t.Helper()
	rootA, rootB := "parallel-root-a@myco.example", "parallel-root-b@myco.example"
	replyA, replyB := "parallel-reply-a@myco.example", "parallel-reply-b@myco.example"
	if scenario.forward {
		return concurrentMail{firstID: replyA, secondID: rootB, first: email(captureOwner, "", "alice@acme.example", replyA, "zz-missing-root@myco.example> <"+rootB), second: email(captureOwner, "", "bob@globex.example", rootB, ""), messages: []string{replyA, rootB}}
	}
	env.syncSent(t, map[string]bool{rootA: true, rootB: true},
		email(captureOwner, "", "alice@acme.example", rootA, ""),
		email(captureOwner, "", "bob@globex.example", rootB, ""))
	refsA, refsB := rootA, rootB
	messages := []string{rootA, rootB, replyA, replyB}
	if scenario.merge {
		branchA, branchB := "parallel-z-branch-a@myco.example", "parallel-z-branch-b@myco.example"
		env.syncSent(t, map[string]bool{branchA: true}, email(captureOwner, "", "alice@acme.example", branchA, ""))
		messages = append(messages, branchA)
		if scenario.overlap {
			branchB = branchA
		} else {
			env.syncSent(t, map[string]bool{branchB: true}, email(captureOwner, "", "bob@globex.example", branchB, ""))
			messages = append(messages, branchB)
		}
		refsA, refsB = rootA+"> <"+branchA, rootB+"> <"+branchB
	}
	if scenario.manyReferences {
		references := []string{refsA}
		for i := range 49 {
			branch := fmt.Sprintf("parallel-z-reference-%02d@myco.example", i)
			env.syncSent(t, map[string]bool{branch: true}, email(captureOwner, "", "alice@acme.example", branch, ""))
			references = append(references, branch)
			messages = append(messages, branch)
		}
		refsA = strings.Join(references, "> <")
	}
	return concurrentMail{firstID: replyA, secondID: replyB, first: email(captureOwner, "", "alice@acme.example", replyA, refsA), second: email(captureOwner, "", "bob@globex.example", replyB, refsB), messages: messages}
}

func independentImports(t *testing.T, scenario concurrentImportCase) {
	t.Helper()
	env := newCaptureEnv(t)
	mail := concurrentMailFixture(t, env, scenario)
	overlap := scenario.overlap
	barrier := &importThreadBarrier{entered: make(chan struct{}), release: make(chan struct{})}
	firstConfig := env.e.Pool.Config()
	firstConfig.ConnConfig.Tracer = multitracer.New(firstConfig.ConnConfig.Tracer, barrier)
	firstPool, err := testdb.OwnPoolFromConfig(t.Context(), firstConfig)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(firstPool.Close)
	secondConfig := env.e.Pool.Config()
	secondConfig.ConnConfig.RuntimeParams["lock_timeout"] = "250ms"
	secondBarrier := &importThreadBarrier{entered: make(chan struct{}), release: make(chan struct{}), started: make(chan struct{})}
	close(secondBarrier.release)
	if overlap {
		secondConfig.ConnConfig.RuntimeParams["lock_timeout"] = "0"
		secondConfig.ConnConfig.Tracer = multitracer.New(secondConfig.ConnConfig.Tracer, secondBarrier)
	}
	secondPool, err := testdb.OwnPoolFromConfig(t.Context(), secondConfig)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(secondPool.Close)
	vault := newTestKeyvault(t, env.e)
	first := compose.NewCaptureRegistry(firstPool, vault, compose.CaptureConfig{HistoryConcurrency: 2})
	second := compose.NewCaptureRegistry(secondPool, vault, compose.CaptureConfig{HistoryConcurrency: 2})
	first.Register(&independentImportPage{mailBatchConnector: &mailBatchConnector{
		raws: [][]byte{mail.first}, sent: map[string]bool{mail.firstID: true},
	}, barrier: barrier})
	secondPage := &independentImportPage{mailBatchConnector: &mailBatchConnector{
		name: "graph",
		raws: [][]byte{mail.second}, sent: map[string]bool{mail.secondID: true},
	}}
	if overlap {
		secondPage.barrier = secondBarrier
	}
	second.Register(secondPage)
	actor := humanWithScopes(env.e, env.e.Rep1, []principal.Scope{principal.ScopeRead})
	if _, err := first.Connect(actor, "gmail", connector.Auth("first")); err != nil {
		t.Fatal(err)
	}
	if _, err := second.Connect(actor, "graph", connector.Auth("second")); err != nil {
		t.Fatal(err)
	}
	user := ids.From[ids.UserKind](env.e.Rep1)
	a, err := first.StartBackfill(actor, "gmail", user, 6, connector.BackfillEstimate{Messages: 1}, enqueueNothing)
	if err != nil {
		t.Fatal(err)
	}
	b, err := second.StartBackfill(actor, "graph", user, 6, connector.BackfillEstimate{Messages: 1}, enqueueNothing)
	if err != nil {
		t.Fatal(err)
	}
	generations := map[ids.UUID]int64{}
	var generationsMu sync.Mutex
	scheduler := capturemod.BackfillScheduler{Enqueue: func(_ context.Context, _ pgx.Tx, id ids.UUID, generation int64, _ time.Time) error {
		generationsMu.Lock()
		defer generationsMu.Unlock()
		generations[id] = generation
		return nil
	}}
	// The second pool's lock_timeout proves the imports do not wait for each
	// other. This deadline only bounds the run, which a loaded runner slows.
	ctx, cancel := context.WithTimeout(principal.WithWorkspaceID(context.Background(), env.e.WS), time.Minute)
	defer cancel()
	if err := first.DispatchBackfills(ctx, scheduler, false); err != nil {
		t.Fatal(err)
	}
	if len(generations) != 2 {
		t.Fatalf("admitted %d imports, want two", len(generations))
	}
	firstGeneration, secondGeneration := generations[a.ID], generations[b.ID]
	finished, workerDone := make(chan error, 1), make(chan struct{})
	go func() {
		defer close(workerDone)
		_, _, _, err := first.RunBackfillExecution(ctx, a.ID, firstGeneration, scheduler)
		finished <- err
	}()
	var releaseOnce sync.Once
	release := func() { releaseOnce.Do(func() { close(barrier.release) }) }
	t.Cleanup(func() { cancel(); release(); <-workerDone })
	select {
	case <-barrier.entered:
	case err := <-finished:
		t.Fatalf("first import missed the thread boundary: %v", err)
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	secondFinished := make(chan error, 1)
	go func() {
		done, completed, _, err := second.RunBackfillExecution(ctx, b.ID, secondGeneration, scheduler)
		if err == nil && (!done || !completed) {
			err = fmt.Errorf("import incomplete: done=%v completed=%v", done, completed)
		}
		secondFinished <- err
	}()
	if overlap {
		select {
		case <-secondBarrier.started:
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		}
		release()
	}
	secondErr := <-secondFinished
	release()
	if secondErr != nil {
		t.Fatalf("second admitted import: %v", secondErr)
	}
	if err := <-finished; err != nil {
		t.Fatal(err)
	}
	var captured, threads int
	if err := env.e.Owner.QueryRow(ctx, `SELECT count(*),count(DISTINCT thread_key) FROM activity WHERE source_id=ANY(@messages)`, pgx.NamedArgs{"messages": mail.messages}).Scan(&captured, &threads); err != nil {
		t.Fatal(err)
	}
	wantThreads := 2
	if overlap {
		wantThreads = 1
	}
	if scenario.overlap {
		expected := "parallel-root-a@myco.example"
		if scenario.forward {
			expected = "parallel-root-b@myco.example"
		}
		var canonical string
		if err := env.e.Owner.QueryRow(ctx, `SELECT thread_key FROM activity WHERE source_id=@message`, pgx.NamedArgs{"message": mail.firstID}).Scan(&canonical); err != nil {
			t.Fatal(err)
		}
		if canonical != expected {
			t.Fatalf("canonical thread=%s, want %s", canonical, expected)
		}
	}
	if captured != len(mail.messages) || threads != wantThreads {
		t.Fatalf("captured messages=%d threads=%d, want %d in %d threads", captured, threads, len(mail.messages), wantThreads)
	}
}

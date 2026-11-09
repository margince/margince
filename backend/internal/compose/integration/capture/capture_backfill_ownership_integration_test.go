// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package capture

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/compose/integration"
	capturemod "github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

type heldHistoryPage struct {
	*mailPageConnector
	entered chan struct{}
	proceed chan struct{}
}

func (p *heldHistoryPage) BackfillPage(ctx context.Context, auth connector.Auth, after time.Time, token string, sink connector.Sink) (connector.BackfillPageResult, error) {
	select {
	case p.entered <- struct{}{}:
	case <-ctx.Done():
		return connector.BackfillPageResult{}, ctx.Err()
	}
	select {
	case <-p.proceed:
	case <-ctx.Done():
		return connector.BackfillPageResult{}, ctx.Err()
	}
	return p.mailPageConnector.BackfillPage(ctx, auth, after, token, sink)
}

func TestAReplacedHistoryExecutionCannotWriteMailOrCompleteItsSuccessor(t *testing.T) {
	e := integration.SetupSearch(t)
	seedCaptureRole(t, e)
	vault := newTestKeyvault(t, e)
	first := compose.NewCaptureRegistry(e.Pool, vault, compose.CaptureConfig{})
	second := compose.NewCaptureRegistry(e.Pool, vault, compose.CaptureConfig{})
	const messageID = "execution-fence@myco.example"
	page := &heldHistoryPage{
		mailPageConnector: &mailPageConnector{raws: [][]byte{email(captureOwner, "", "recipient@globex.example", messageID, "")}, sent: map[string]bool{messageID: true}},
		entered:           make(chan struct{}, 2), proceed: make(chan struct{}),
	}
	var releaseOnce sync.Once
	release := func() { releaseOnce.Do(func() { close(page.proceed) }) }
	t.Cleanup(release)
	first.Register(page)
	second.Register(page)
	actor := humanWithScopes(e, e.Rep1, []principal.Scope{principal.ScopeRead})
	if _, err := first.Connect(actor, "gmail", connector.Auth("refresh")); err != nil {
		t.Fatal(err)
	}
	run, err := first.StartBackfill(actor, "gmail", ids.From[ids.UserKind](e.Rep1), 3, connector.BackfillEstimate{Messages: 1}, enqueueNothing)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(principal.WithWorkspaceID(context.Background(), e.WS))
	defer cancel()
	var latest atomic.Int64
	scheduler := capturemod.BackfillScheduler{
		Enqueue: func(_ context.Context, _ pgx.Tx, _ ids.UUID, generation int64, _ time.Time) error {
			latest.Store(generation)
			return nil
		},
		Executing: func(context.Context, pgx.Tx, ids.UUID, int64) (bool, error) { return false, nil },
	}
	if err := first.DispatchBackfills(ctx, scheduler, false); err != nil {
		t.Fatal(err)
	}
	generation := latest.Load()
	finished := make(chan error, 1)
	stopped := make(chan struct{})
	go func() {
		defer close(stopped)
		_, _, _, err := first.RunBackfillExecution(ctx, run.ID, generation, scheduler)
		finished <- err
	}()
	t.Cleanup(func() { cancel(); release(); <-stopped })
	select {
	case <-page.entered:
	case err := <-finished:
		t.Fatalf("page never reached provider: %v", err)
	}
	if err := second.DispatchBackfills(ctx, scheduler, true); err != nil {
		t.Fatal(err)
	}
	if latest.Load() != generation {
		t.Fatal("recovery replaced a live execution")
	}
	args := []any{run.ID}
	var pid int32
	if err := e.Owner.QueryRow(ctx, fmt.Sprintf("SELECT execution_owner_pid FROM capture_backfill WHERE id=$%d", len(args)), args...).Scan(&pid); err != nil {
		t.Fatal(err)
	}
	args = []any{pid}
	var terminated bool
	if err := e.Owner.QueryRow(ctx, fmt.Sprintf("SELECT pg_terminate_backend($%d, 5000)", len(args)), args...).Scan(&terminated); err != nil || !terminated {
		t.Fatalf("terminate: %v %v", terminated, err)
	}
	if err := second.DispatchBackfills(ctx, scheduler, true); err != nil {
		t.Fatal(err)
	}
	replacement := latest.Load()
	if replacement <= generation {
		t.Fatalf("lost execution not superseded: %d to %d", generation, replacement)
	}
	release()
	if err := <-finished; err == nil {
		t.Fatal("stale page reported success")
	}
	args = []any{messageID}
	var count int
	if err := e.Owner.QueryRow(ctx, fmt.Sprintf("SELECT count(*) FROM activity WHERE source_id=$%d", len(args)), args...).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("stale page wrote %d emails", count)
	}
	args = []any{run.ID}
	var status string
	var stored int64
	if err := e.Owner.QueryRow(ctx, fmt.Sprintf("SELECT status,execution_generation FROM capture_backfill WHERE id=$%d", len(args)), args...).Scan(&status, &stored); err != nil {
		t.Fatal(err)
	}
	if status != "running" || stored != replacement {
		t.Fatalf("stale page changed its successor: %s %d", status, stored)
	}
	done, completed, _, err := second.RunBackfillExecution(ctx, run.ID, replacement, scheduler)
	if err != nil || !done || !completed {
		t.Fatalf("replacement: done=%v complete=%v err=%v", done, completed, err)
	}
	args = []any{messageID}
	if err := e.Owner.QueryRow(ctx, fmt.Sprintf("SELECT count(*) FROM activity WHERE source_id=$%d", len(args)), args...).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("replacement captured %d emails", count)
	}
}

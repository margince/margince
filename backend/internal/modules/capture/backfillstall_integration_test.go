// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package capture_test

// A message whose fetch fails on consecutive attempts is walked past on the
// next one. Permanently broken messages cannot stall a run.

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

type stalledHistory struct {
	fixtureConnector
	skipped [][]string
	bad     []string // messages whose fetch fails until the page is told to walk past them
}

func (*stalledHistory) AccountLabel(auth connector.Auth) (string, error) {
	return string(auth) + "@mail.example", nil
}

func (*stalledHistory) EstimateBackfill(context.Context, connector.Auth, time.Time) (connector.BackfillEstimate, error) {
	return connector.BackfillEstimate{Messages: 3}, nil
}

func (p *stalledHistory) BackfillPage(ctx context.Context, _ connector.Auth, _ time.Time, _ string, _ connector.Sink) (connector.BackfillPageResult, error) {
	skip := connector.BackfillSkipFrom(ctx)
	p.skipped = append(p.skipped, skip)
	for _, id := range p.bad {
		if !slices.Contains(skip, id) {
			return connector.BackfillPageResult{}, &connector.MessageFaultError{ID: id, Err: connector.ErrUnreachable}
		}
	}
	return connector.BackfillPageResult{Scanned: 3, Captured: 3 - len(p.bad), Failed: len(p.bad)}, nil
}

func TestAMessageThatFailsThreeAttemptsInARowIsSkippedAndCounted(t *testing.T) {
	t.Parallel()
	provider := &stalledHistory{bad: []string{"bad-message"}}
	ctx, reg, id := admissionRunWith(t, provider)
	queue := &recordedBackfillQueue{generations: map[ids.UUID]int64{}}
	if err := reg.DispatchBackfills(ctx, queue.scheduler(), false); err != nil {
		t.Fatal(err)
	}
	owner, _ := setupCaptureDB(t)
	for attempt := 1; attempt <= 3; attempt++ {
		if _, _, _, err := reg.RunBackfillExecution(ctx, id, queue.generations[id], queue.scheduler()); !errors.Is(err, connector.ErrUnreachable) {
			t.Fatalf("attempt %d: %v", attempt, err)
		}
		if _, err := owner.Exec(ctx, `UPDATE capture_backfill SET execution_due_at=now()-interval '1 second' WHERE id=@run`, pgx.NamedArgs{"run": id}); err != nil {
			t.Fatal(err)
		}
	}
	if _, completed, _, err := reg.RunBackfillExecution(ctx, id, queue.generations[id], queue.scheduler()); err != nil || !completed {
		t.Fatalf("fourth attempt completed=%v: %v", completed, err)
	}
	if want := [][]string{nil, nil, nil, {"bad-message"}}; !slices.EqualFunc(provider.skipped, want, slices.Equal) {
		t.Fatalf("skip ids per attempt = %q, want %q", provider.skipped, want)
	}
	var failed int
	if err := owner.QueryRow(ctx, `SELECT failed FROM capture_backfill WHERE id=@run`, pgx.NamedArgs{"run": id}).Scan(&failed); err != nil || failed != 1 {
		t.Fatalf("failed = %d (%v), want the skipped message counted once", failed, err)
	}
}

// Two messages both fail every attempt. The first is bypassed after its
// strikes, then the second fails on its own. Neither loses its strikes before
// the page commits.
func TestTwoPermanentlyFailingMessagesAreBothSkippedAndThePageCommits(t *testing.T) {
	t.Parallel()
	provider := &stalledHistory{bad: []string{"bad-a", "bad-b"}}
	ctx, reg, id := admissionRunWith(t, provider)
	queue := &recordedBackfillQueue{generations: map[ids.UUID]int64{}}
	if err := reg.DispatchBackfills(ctx, queue.scheduler(), false); err != nil {
		t.Fatal(err)
	}
	owner, _ := setupCaptureDB(t)
	for attempt := 1; attempt <= 6; attempt++ {
		if _, _, _, err := reg.RunBackfillExecution(ctx, id, queue.generations[id], queue.scheduler()); !errors.Is(err, connector.ErrUnreachable) {
			t.Fatalf("attempt %d: %v", attempt, err)
		}
		if _, err := owner.Exec(ctx, `UPDATE capture_backfill SET execution_due_at=now()-interval '1 second' WHERE id=@run`, pgx.NamedArgs{"run": id}); err != nil {
			t.Fatal(err)
		}
	}
	if _, completed, _, err := reg.RunBackfillExecution(ctx, id, queue.generations[id], queue.scheduler()); err != nil || !completed {
		t.Fatalf("seventh attempt completed=%v: %v", completed, err)
	}
	if last := provider.skipped[len(provider.skipped)-1]; !slices.Equal(last, []string{"bad-a", "bad-b"}) {
		t.Fatalf("last attempt skip ids = %q, want both messages", last)
	}
	var failed int
	if err := owner.QueryRow(ctx, `SELECT failed FROM capture_backfill WHERE id=@run`, pgx.NamedArgs{"run": id}).Scan(&failed); err != nil || failed != 2 {
		t.Fatalf("failed = %d (%v), want both skipped messages counted", failed, err)
	}
}

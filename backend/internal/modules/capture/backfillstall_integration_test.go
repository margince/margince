// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package capture_test

// A message whose fetch fails on consecutive attempts is walked past on the
// next one. One permanently broken message cannot stall a run.

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

type stalledHistory struct {
	fixtureConnector
	skipped []string
}

func (*stalledHistory) AccountLabel(auth connector.Auth) (string, error) {
	return string(auth) + "@mail.example", nil
}

func (*stalledHistory) EstimateBackfill(context.Context, connector.Auth, time.Time) (connector.BackfillEstimate, error) {
	return connector.BackfillEstimate{Messages: 3}, nil
}

func (p *stalledHistory) BackfillPage(ctx context.Context, _ connector.Auth, _ time.Time, _ string, _ connector.Sink) (connector.BackfillPageResult, error) {
	id := connector.BackfillSkipFrom(ctx)
	p.skipped = append(p.skipped, id)
	if id == "bad-message" {
		return connector.BackfillPageResult{Scanned: 3, Captured: 2, Failed: 1}, nil
	}
	return connector.BackfillPageResult{}, &connector.MessageFaultError{ID: "bad-message", Err: connector.ErrUnreachable}
}

func TestAMessageThatFailsThreeAttemptsInARowIsSkippedAndCounted(t *testing.T) {
	provider := &stalledHistory{}
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
	if want := []string{"", "", "", "bad-message"}; len(provider.skipped) != 4 || provider.skipped[3] != want[3] || provider.skipped[2] != "" {
		t.Fatalf("skip ids per attempt = %q, want %q", provider.skipped, want)
	}
	var failed int
	if err := owner.QueryRow(ctx, `SELECT failed FROM capture_backfill WHERE id=@run`, pgx.NamedArgs{"run": id}).Scan(&failed); err != nil || failed != 1 {
		t.Fatalf("failed = %d (%v), want the skipped message counted once", failed, err)
	}
}

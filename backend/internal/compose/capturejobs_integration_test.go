// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// The overnight capture pair rides River (ADR-0063). With brains and a
// registry configured, NewJobRunner registers the classify and enrich jobs
// and their RunOnStart passes complete, through the real worker adapters.

import (
	"context"
	"io"
	"log/slog"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/compose/integration"
)

// countJobsOfKind answers how many river_job rows of a kind exist right now.
// The database is the test's own, so the count is this test's inserts and
// nothing else.
func countJobsOfKind(ctx context.Context, t *testing.T, pool *pgxpool.Pool, kind string) int {
	t.Helper()
	var n int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM river_job WHERE kind = $1`, kind).Scan(&n); err != nil {
		t.Fatalf("counting %s rows: %v", kind, err)
	}
	return n
}

func TestCaptureOvernightJobsRegisterAndRun(t *testing.T) {
	t.Parallel()
	b := setupBackfillWire(t)
	quiet := slog.New(slog.NewTextHandler(io.Discard, nil))

	runner, err := NewJobRunner(b.env.Pool, quiet, JobRunnerConfig{
		GmailRegistry: b.registry,
		// Zero-value scripts: the classify brain labels whatever backlog
		// the fake connector synced; the enrich pass finds no
		// connector-created contact (this registry wires no ensurer) and
		// completes as an honest no-op.
		ClassifyBrain: &scriptedClassifyBrain{},
		EnrichBrain:   &signatureScriptBrain{},
	})
	if err != nil {
		t.Fatalf("NewJobRunner: %v", err)
	}
	sub, cancelSub := runner.SubscribeCompleted()
	defer cancelSub()

	ctx := context.Background()
	if err := runner.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer integration.StartedJobRunner(t, runner)()

	// Awaited as a set, not a sequence. Both are RunOnStart, so River enqueues
	// them together and finishes them in whatever order the queue allows.
	awaitKindsCompleted(t, sub, "capture_classify", "capture_enrich")
}

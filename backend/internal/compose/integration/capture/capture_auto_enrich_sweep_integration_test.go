// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package capture

// The captured-organization auto-enrich sweep over the REAL River runner
// (ADR-0072/A118): compose.NewJobRunner registers the sweep RunOnStart exactly
// as cmd/worker wires it, so Start fires one pass. Against a domain-named
// captured org with the flag on, that pass must create a system-requested
// dossier, enqueue its deep read, arm the cursor, and reserve a daily-cap slot —
// the whole trigger path end to end, observed on River's own completion channel
// (no sleep, no poll). The brain is nil on purpose: the deep read the sweep
// enqueues then fails honestly, but the sweep's own job still completes, and
// what this proves is the TRIGGER, not the read (the auto-apply lane has its own
// test).

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestCaptureAutoEnrichSweepTriggersADeepReadForACapturedOrg(t *testing.T) {
	e := integration.Setup(t)
	orgID := ids.NewV7()
	// A captured, domain-named org (name_source='domain') with a live primary
	// domain — the shape the sweep enriches. The capture_auto_enrich flag is ON
	// by default (migration 0121), so no toggle is needed.
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(context.Background(), `
			INSERT INTO organization (id, owner_id, display_name, name_source, source, captured_by)
			VALUES ($1, $2, 'Gitex', 'domain', 'connector:gmail', 'connector:gmail')`,
			orgID, e.Rep1); err != nil {
			return err
		}
		_, err := tx.Exec(context.Background(), `
			INSERT INTO organization_domain (organization_id, domain, is_primary, source, captured_by)
			VALUES ($1, 'gitex.com', true, 'connector:gmail', 'connector:gmail')`, orgID)
		return err
	}); err != nil {
		t.Fatalf("seeding the captured org: %v", err)
	}

	integration.ApplyRiverSchema(t)
	quiet := slog.New(slog.NewTextHandler(io.Discard, nil))
	runner, err := compose.NewJobRunner(e.Pool, quiet, compose.JobRunnerConfig{
		CloseDateInterval: time.Hour,
		ReconcileInterval: time.Hour,
		TimeScanInterval:  time.Hour,
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
	defer func() {
		stopCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if err := runner.Stop(stopCtx); err != nil {
			t.Errorf("Stop: %v", err)
		}
	}()

	waitCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	// THE PASS ITSELF, which is now the only kind there is (ADR-0103). This
	// used to wait on the workspace child, because a dispatcher completed as
	// soon as its fan-out was enqueued and waiting on it raced the work. A
	// collapsed pass completes when the work is done, so the thing to wait for
	// and the thing that does the work are the same row.
	integration.AwaitKindCompleted(waitCtx, t, sub, compose.CaptureAutoEnrichSweepArgs{}.Kind())

	// The sweep created a system-requested dossier for the org...
	var readCount int
	var requestedBy string
	var readID string
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(),
			`SELECT count(*), coalesce(max(requested_by), ''), coalesce(max(id::text), '')
			 FROM site_read WHERE organization_id = $1`,
			orgID).Scan(&readCount, &requestedBy, &readID)
	}); err != nil {
		t.Fatalf("reading the dossier: %v", err)
	}
	if readCount != 1 || requestedBy != "system:capture_auto_enrich" {
		t.Fatalf("dossier count=%d requested_by=%q, want 1 / system:capture_auto_enrich", readCount, requestedBy)
	}

	// ...at the housekeeping priority (River's lowest tier) — a boot-time fan-out
	// across every due org must never queue ahead of a live, human-started read
	// sharing deep_read's two workers. Scoped to THIS dossier's own job by
	// site_read_id: sweepWorkspace also runs sweepDomainTriage unconditionally
	// before the auto-enrich loop, which can enqueue its own site_deep_read row
	// for an unrelated domain in the same pass — an unscoped query naming only
	// `kind` would read whichever row Postgres happened to return first.
	var priority int
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(),
			`SELECT priority FROM river_job WHERE kind = $1 AND args ->> 'site_read_id' = $2`,
			compose.SiteDeepReadArgs{}.Kind(), readID,
		).Scan(&priority)
	}); err != nil {
		t.Fatalf("reading the enqueued read's priority: %v", err)
	}
	if priority != compose.DeepReadPriorityHousekeeping {
		t.Fatalf("sweep-enqueued deep read priority = %d, want %d (housekeeping)", priority, compose.DeepReadPriorityHousekeeping)
	}

	// ...and armed the cursor (attempt counted, outcome queued)...
	var attempts int
	var outcome string
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(),
			`SELECT attempts, last_outcome FROM capture_auto_enrich_state WHERE organization_id = $1`,
			orgID).Scan(&attempts, &outcome)
	}); err != nil {
		t.Fatalf("reading the cursor: %v", err)
	}
	if attempts != 1 || outcome != "queued" {
		t.Fatalf("cursor attempts=%d outcome=%q, want 1 / queued", attempts, outcome)
	}

	// ...and reserved exactly one daily-cap slot for this workspace's UTC day.
	var enqueued int
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(),
			`SELECT coalesce(sum(enqueued), 0) FROM capture_auto_enrich_budget`).Scan(&enqueued)
	}); err != nil {
		t.Fatalf("reading the budget: %v", err)
	}
	if enqueued != 1 {
		t.Fatalf("reserved %d cap slots, want exactly 1", enqueued)
	}
}

// TestCaptureAutoEnrichSweepQueuesDomainTriageAtHousekeepingPriorityToo proves
// the same housekeeping priority for the OTHER sweep-sourced caller sharing
// siteDeepReadInsertOpts, over the same real River runner this file's sibling
// test uses. sweepWorkspace runs sweepDomainTriage unconditionally before the
// auto-enrich loop (captureautoenrich.go), so an open domain question with no
// captured org queues through startDomainTriageRead, not startAutoEnrichRead —
// the call site capturedomaintriage_integration_test.go's own header comment
// names as untestable without an ambient River client this runner now supplies.
func TestCaptureAutoEnrichSweepQueuesDomainTriageAtHousekeepingPriorityToo(t *testing.T) {
	e := integration.Setup(t)
	const domain = "unjudged.example"
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		_, err := tx.Exec(context.Background(), `
			INSERT INTO organization_domain_disposition (domain, status, owner_id)
			VALUES ($1, 'pending', $2)`, domain, e.Rep1)
		return err
	}); err != nil {
		t.Fatalf("seeding the open domain question: %v", err)
	}

	integration.ApplyRiverSchema(t)
	quiet := slog.New(slog.NewTextHandler(io.Discard, nil))
	runner, err := compose.NewJobRunner(e.Pool, quiet, compose.JobRunnerConfig{
		CloseDateInterval: time.Hour,
		ReconcileInterval: time.Hour,
		TimeScanInterval:  time.Hour,
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
	defer func() {
		stopCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if err := runner.Stop(stopCtx); err != nil {
			t.Errorf("Stop: %v", err)
		}
	}()

	waitCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	integration.AwaitKindCompleted(waitCtx, t, sub, compose.CaptureAutoEnrichSweepArgs{}.Kind())

	var readID string
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(),
			`SELECT id::text FROM site_read WHERE seed_url = $1 AND target_kind = 'domain_triage'`,
			"https://"+domain,
		).Scan(&readID)
	}); err != nil {
		t.Fatalf("reading the triage dossier: %v", err)
	}

	var priority int
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(),
			`SELECT priority FROM river_job WHERE kind = $1 AND args ->> 'site_read_id' = $2`,
			compose.SiteDeepReadArgs{}.Kind(), readID,
		).Scan(&priority)
	}); err != nil {
		t.Fatalf("reading the triage read's priority: %v", err)
	}
	if priority != compose.DeepReadPriorityHousekeeping {
		t.Fatalf("sweep-enqueued domain triage read priority = %d, want %d (housekeeping)", priority, compose.DeepReadPriorityHousekeeping)
	}
}

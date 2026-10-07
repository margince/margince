// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/platform/jobs"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// Raising the allowance is how an administrator releases work held for budget,
// so a held account scan runs again: the resume pass answers without error and
// the scan's clock and its job both move up.
func TestRaisingTheAllowanceWakesAHeldAccountScan(t *testing.T) {
	e := integration.Setup(t)
	grantBudgetRequester(t, e, e.Rep1)
	company := e.SeedCompany(t, "Held account", &e.Rep1)
	next := time.Date(2099, 1, 1, 0, 0, 0, 0, time.UTC)
	scanID := ids.NewV7()
	err := e.DB().Tx(e.Admin(), func(tx pgx.Tx) error {
		_, err := tx.Exec(e.Admin(), `INSERT INTO company_scan
			(id, user_id, company_id, status, degrade_reason, next_attempt_at, attempt)
			VALUES ($1, $2, $3, 'queued', 'budget_deferred', $4, 1)`, scanID, e.Rep1, company, next)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	runner, err := jobs.NewInserter(e.Pool, slog.New(slog.DiscardHandler))
	if err != nil {
		t.Fatal(err)
	}
	if err := runner.Enqueue(e.Admin(), AccountScanArgs{
		Workspace: e.WS, CompanyID: company, ScanID: scanID, ViewerID: e.Rep1,
	}, &river.InsertOpts{ScheduledAt: next, MaxAttempts: 3}); err != nil {
		t.Fatal(err)
	}

	setRecoveryAllowance(t, e, 1000)
	recovery := newAIBudgetResumeWorker(e.Pool, slog.New(slog.DiscardHandler))
	// The context the worker's periodic pass hands over: no actor, no request
	// and so no correlation id. A fixture that supplied one would pass over the
	// failure this test exists for.
	workerCtx := context.Background()
	for pass := 1; pass <= 2; pass++ {
		if err := recovery.resumeWorkspace(workerCtx, e.WS); err != nil {
			t.Fatalf("resume pass %d failed: %v", pass, err)
		}
	}

	var clock, scheduled time.Time
	err = e.DB().Tx(e.Admin(), func(tx pgx.Tx) error {
		if err := tx.QueryRow(e.Admin(), `SELECT next_attempt_at FROM company_scan WHERE id = $1`, scanID).Scan(&clock); err != nil {
			return err
		}
		return tx.QueryRow(e.Admin(), `SELECT scheduled_at FROM river_job WHERE kind = 'account_scan' AND args->>'scan_id' = $1`, scanID.String()).Scan(&scheduled)
	})
	if err != nil {
		t.Fatal(err)
	}
	if !clock.Before(next) || !scheduled.Before(next) {
		t.Errorf("the held scan still waits for the month: clock=%s job=%s", clock, scheduled)
	}
}

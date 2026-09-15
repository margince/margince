// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package companyscan

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestBudgetRecoveryPreservesTheAccountScanRequest(t *testing.T) {
	e := integration.Setup(t)
	ctx := e.Admin()
	company := e.SeedCompany(t, "Recovery account", &e.Rep1)
	var deferred row
	next := time.Date(2099, 1, 1, 0, 0, 0, 0, time.UTC)
	if err := e.DB().Tx(ctx, func(tx pgx.Tx) error {
		queued, created, err := queue(ctx, tx, ids.From[ids.UserKind](e.Rep1), ids.From[ids.CompanyKind](company))
		if err != nil {
			return err
		}
		if !created {
			t.Fatal("fixture did not queue the scan")
		}
		running, claimed, err := begin(ctx, tx, queued.ID)
		if err != nil {
			return err
		}
		if !claimed {
			t.Fatal("fixture did not claim the scan")
		}
		if err := deferBudget(ctx, tx, running.claim(), next); err != nil {
			return err
		}
		deferred, _, err = load(ctx, tx, ids.From[ids.UserKind](e.Rep1), ids.From[ids.CompanyKind](company))
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if deferred.DegradeReason == nil || *deferred.DegradeReason != "budget_deferred" {
		t.Fatal("deferral did not persist its budget cause")
	}
	offered := 0
	for range 2 {
		if err := ResumeBudgetScans(ctx, e.DB(), func(_ context.Context, _ pgx.Tx, id, requester, parent ids.UUID) (bool, error) {
			offered++
			if id != deferred.ID || requester != e.Rep1 || parent != company {
				t.Fatal("recovery changed the request")
			}
			return true, nil
		}); err != nil {
			t.Fatal(err)
		}
	}
	if offered != 1 {
		t.Fatalf("offered %d times", offered)
	}
	if err := e.DB().Tx(ctx, func(tx pgx.Tx) error {
		current, exists, err := load(ctx, tx, ids.From[ids.UserKind](e.Rep1), ids.From[ids.CompanyKind](company))
		if err != nil {
			return err
		}
		if !exists || current.Attempt != deferred.Attempt || current.NextAttemptAt == nil || !current.NextAttemptAt.Before(next) {
			t.Fatalf("recovered %+v", current)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	// A queued retry clock with another cause is not owned by budget recovery.
	if err := e.DB().Tx(ctx, func(tx pgx.Tx) error {
		args := []any{next, deferred.ID}
		query := fmt.Sprintf(`UPDATE company_scan SET next_attempt_at=$%d,degrade_reason='provider_backoff' WHERE id=$%d`, len(args)-1, len(args))
		_, err := tx.Exec(ctx, query, args...)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if err := ResumeBudgetScans(ctx, e.DB(), func(context.Context, pgx.Tx, ids.UUID, ids.UUID, ids.UUID) (bool, error) {
		t.Fatal("budget recovery selected a provider retry")
		return false, nil
	}); err != nil {
		t.Fatal(err)
	}
	if offered != 1 {
		t.Fatal("provider retry was offered to budget recovery")
	}
}

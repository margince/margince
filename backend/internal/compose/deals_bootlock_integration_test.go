// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/platform/config"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/deployconfig"
)

// Maintenance switched off at boot stops the sweep's write over a saved row
// that leaves it on. Not parallel: the binding is the process's own.
func TestABootSetMaintenanceOffStopsTheSweepsWrite(t *testing.T) {
	e := setupCloseDate(t)
	e.seedSweepDeal(t, "Overdue with maintenance off at boot", e.early, nil, new(-12), 3)
	UseConfig(bootConfigForTest(t, config.RoleWorker, "MARGINCE_DEALS_MAINTENANCE_WRITES_ENABLED=false"))
	t.Cleanup(func() { UseConfig(nil) })

	if err := e.sweep(); err != nil {
		t.Fatal(err)
	}
	swept := e.readSwept(t, e.firstDealID(t))
	if swept.expectedClose == nil || !swept.expectedClose.Before(today()) {
		t.Errorf("the deal was re-dated to %v with maintenance switched off at boot", swept.expectedClose)
	}
}

// A deployment's seed of legal review off writes no row under a boot value,
// which would act only once that value is gone.
func TestABootSetLegalReviewIsNotSeeded(t *testing.T) {
	e := integration.Setup(t)
	bindForTest(t, "MARGINCE_DEALS_LEGAL_REVIEW_ENABLED=true")
	off := false
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		return seedLegalReview(e.Admin(), tx, deployconfig.Seeds{LegalReview: &off})
	}); err != nil {
		t.Fatalf("seeding legal review off under a boot value: %v", err)
	}
	var rows int
	if err := e.Pool.QueryRow(context.Background(),
		`SELECT count(*) FROM setting WHERE key = $1`, deals.LegalReviewEnabled.Key()).Scan(&rows); err != nil {
		t.Fatal(err)
	}
	if rows != 0 {
		t.Errorf("the seed wrote %d legal review rows under a boot value, want none", rows)
	}
}

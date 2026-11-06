// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"errors"
	"github.com/jackc/pgx/v5"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"testing"

	"github.com/margince/margince/backend/internal/modules/agents/runner"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestRiskViewsNarrowToTheNamedOwnerBeforeTheirScanCaps(t *testing.T) {
	f := newSweepFixture(t, 90)
	var pipeline ids.PipelineID
	var stage ids.StageID
	if err := f.e.DB().Tx(f.e.Admin(), func(tx pgx.Tx) error {
		return tx.QueryRow(f.e.Admin(), `SELECT pipeline_id, stage_id FROM deal WHERE id = $1`, f.owned[f.e.Rep1]).Scan(&pipeline, &stage)
	}); err != nil {
		t.Fatal(err)
	}
	for range slippingScanLimit + 1 {
		f.e.SeedDeal(t, "Other owner's newer deal", pipeline, stage, &f.e.Rep2)
	}
	owner := f.e.Rep1
	listed, err := slippingLister(f.e.Pool)(f.e.Admin(), runner.SweepQuietDays, &owner)
	if err != nil {
		t.Fatal(err)
	}
	if len(listed) != 1 || listed[0].DealID != f.owned[owner] {
		t.Fatalf("slipping = %+v, want only the named owner's older deal", listed)
	}
	risk, err := atRiskLister(f.e.Pool, contacts.NewStore(InstallationDB(f.e.Pool)))(f.e.Admin(), &owner)
	if err != nil {
		t.Fatal(err)
	}
	if risk.DealsScanned != 1 || risk.Truncated {
		t.Fatalf("at risk scanned %d, truncated %v; want the owner's one deal", risk.DealsScanned, risk.Truncated)
	}
}

func TestRiskViewsCannotWidenAScheduledMembersBook(t *testing.T) {
	f := newSweepFixture(t, 90)
	ctx := f.run(t, f.e.Rep1)
	other := f.e.Rep2
	listed, err := slippingLister(f.e.Pool)(ctx, runner.SweepQuietDays, &other)
	if err != nil {
		t.Fatal(err)
	}
	if len(listed) != 0 {
		t.Fatalf("another member's slipping deals leaked into the run: %+v", listed)
	}
	risk, err := atRiskLister(f.e.Pool, contacts.NewStore(InstallationDB(f.e.Pool)))(ctx, &other)
	if err != nil {
		t.Fatal(err)
	}
	if risk.DealsScanned != 0 || len(risk.Deals) != 0 {
		t.Fatalf("another member's at-risk deals leaked into the run: %+v", risk)
	}
	missing := ids.NewV7()
	listed, err = slippingLister(f.e.Pool)(f.e.Admin(), runner.SweepQuietDays, &missing)
	if err != nil || len(listed) != 0 {
		t.Fatalf("unknown owner yielded %d deals: %v", len(listed), err)
	}
}

func TestRiskViewsKeepTheDealReadGrant(t *testing.T) {
	f := newSweepFixture(t, 90)
	ctx := f.e.As(f.e.Rep1, nil, principal.Permissions{RowScope: principal.RowScopeOwn})
	owner := f.e.Rep2
	_, err := slippingLister(f.e.Pool)(ctx, runner.SweepQuietDays, &owner)
	if !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Fatalf("slipping without deal read: %v", err)
	}
	_, err = atRiskLister(f.e.Pool, contacts.NewStore(InstallationDB(f.e.Pool)))(ctx, &owner)
	if !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Fatalf("at risk without deal read: %v", err)
	}
}

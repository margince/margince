// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package lifecyclereview

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/contracts"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/modules/projects"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func Reconcile(ctx context.Context, tx pgx.Tx, company ids.CompanyID, now time.Time) (Finding, error) {
	return reconcile(ctx, tx, company, now, false)
}

func ReconcileForDecision(ctx context.Context, tx pgx.Tx, company ids.CompanyID, now time.Time) (Finding, error) {
	if _, err := contacts.LockCompanyTx(ctx, tx, company); err != nil {
		return Finding{}, err
	}
	return reconcile(ctx, tx, company, now, true)
}

func reconcile(ctx context.Context, tx pgx.Tx, company ids.CompanyID, now time.Time, lock bool) (Finding, error) {
	finding, err := latest(ctx, tx, company, lock)
	if finding.OccurredAt.After(now) {
		finding.Incomplete = true
	}
	if err != nil || !Actionable(finding) {
		return finding, err
	}
	zone, err := identity.TimezoneAppliedTx(ctx, tx)
	if err != nil {
		return Finding{}, err
	}
	location, err := storekit.LoadZone(zone)
	if err != nil {
		return Finding{}, err
	}
	active, complete, err := contracts.CompanyContinuityTx(ctx, tx, company, storekit.WorkspaceDay(now, location), lock)
	if err != nil {
		return Finding{}, err
	}
	finding.ActiveContract = active
	finding.Incomplete = finding.Incomplete || !complete
	active, complete, err = projects.CompanyDeliveryTx(ctx, tx, company, lock)
	if err != nil {
		return Finding{}, err
	}
	finding.ActiveDelivery = active
	finding.Incomplete = finding.Incomplete || !complete
	return finding, nil
}

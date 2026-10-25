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
	"github.com/margince/margince/backend/internal/shared/kernel/occurredday"
)

// Reconcile keeps ongoing agreements and delivery from becoming account endings.
func Reconcile(ctx context.Context, tx pgx.Tx, company ids.CompanyID, now time.Time) (Finding, error) {
	return reconcileContinuity(ctx, tx, company, now, false)
}

// ReconcileForDecision locks account evidence before a lifecycle mutation.
func ReconcileForDecision(ctx context.Context, tx pgx.Tx, company ids.CompanyID, now time.Time) (Finding, error) {
	if _, err := contacts.LockCompanyTx(ctx, tx, company); err != nil {
		return Finding{}, err
	}
	return reconcileContinuity(ctx, tx, company, now, true)
}

func reconcileContinuity(ctx context.Context, tx pgx.Tx, company ids.CompanyID, now time.Time, lock bool) (Finding, error) {
	finding, err := latest(ctx, tx, company, lock)
	if err != nil {
		return finding, err
	}
	zone, err := identity.ZoneAppliedTx(ctx, tx)
	if err != nil {
		return finding, err
	}
	if !occurredday.Begun(occurredday.Moment{At: finding.OccurredAt, DateOnly: finding.DateOnly}, now, zone) {
		finding.Incomplete = true
	}
	if !Actionable(finding) {
		return finding, nil
	}
	location, err := identity.ZoneAppliedTx(ctx, tx)
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

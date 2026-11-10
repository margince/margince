// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package deals

// What a deal needs to ask about the contracts filed against it, as a port
// compose fills. `contracts` owns the contract table and a module never imports
// a sibling (ADR-0054), so the edge is injected like EnsurePartner and
// EnsureProjectAttachable.

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// EnsureContractsShareCompany refuses to move a deal to a company that the
// contracts filed against it do not name. A contract on a deal is visible
// through that deal alone, so the move would show them to the new company.
//
// It runs after updateDealInTx locked the deal row. A concurrent contract
// filing reads the deal under a share lock, so one of the two waits.
type EnsureContractsShareCompany func(ctx context.Context, tx pgx.Tx, dealID ids.DealID, companyID ids.CompanyID) error

// refusingEnsureContractsShareCompany is what an un-injected check becomes. It
// refuses every company move, because a check that failed open would let one
// company's contracts reach another company's readers.
func refusingEnsureContractsShareCompany() EnsureContractsShareCompany {
	return func(context.Context, pgx.Tx, ids.DealID, ids.CompanyID) error {
		return errors.New("deals: the EnsureContractsShareCompany seam was not injected; " +
			"construct this store with installseam.Deals(), which binds contracts's " +
			"EnsureDealContractsShareCompany")
	}
}

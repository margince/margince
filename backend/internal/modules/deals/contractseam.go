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
// contracts filed against it do not name.
//
// A contract with a deal is visible through that deal alone
// (contracts.VisibleClause). If a deal moved from company A to company B kept
// A's contracts, everyone who can see the deal would read A's contracts. So a
// company move is refused while such contracts are attached; the user detaches
// them or files them against a deal of the right company first.
//
// It runs in the caller's transaction, after updateDealInTx has locked the deal
// row. A contract filed against the deal at the same time reads the deal's
// company under a share lock (auth.EnsureLinkTarget), so one of the two waits
// and sees what the other committed.
type EnsureContractsShareCompany func(ctx context.Context, tx pgx.Tx, dealID ids.DealID, companyID ids.CompanyID) error

// refusingEnsureContractsShareCompany is what an un-injected check becomes: it
// refuses every company move rather than admitting every one, because a check
// that failed open would let one company's contracts reach another company's readers.
func refusingEnsureContractsShareCompany() EnsureContractsShareCompany {
	return func(context.Context, pgx.Tx, ids.DealID, ids.CompanyID) error {
		return errors.New("deals: the EnsureContractsShareCompany seam was not injected; " +
			"construct this store with installseam.Deals(), which binds contracts's " +
			"EnsureDealContractsShareCompany")
	}
}

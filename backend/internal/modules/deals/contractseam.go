// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package deals

// What a deal needs from the agreements filed against it, as a port the
// composition root fills (a module never imports a sibling).

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// EnsureContractsShareCompany refuses to move a deal to a company its own
// agreements do not name.
//
// The leak is in the contract read. A contract with a deal is visible through
// that deal alone, so agreements on deals a caller cannot see stay hidden.
// A deal moved to company B would then publish company A's agreements to
// everyone who can see it. Only comparing the two companies catches that.
//
// It runs in the caller's transaction, after the deal row is locked. A contract
// filed against this deal at the same time reads its company under a share
// lock. One of the two waits and sees what the other decided.
type EnsureContractsShareCompany func(ctx context.Context, tx pgx.Tx, dealID ids.DealID, companyID ids.CompanyID) error

// refusingEnsureContractsShareCompany is what an un-injected check becomes: it
// refuses every move rather than admitting every one. A seam that failed OPEN
// here would silently restore the hole it exists to close.
func refusingEnsureContractsShareCompany() EnsureContractsShareCompany {
	return func(context.Context, pgx.Tx, ids.DealID, ids.CompanyID) error {
		return errors.New("deals: the EnsureContractsShareCompany seam was not injected; " +
			"construct this store with installseam.Deals(), which binds contracts's " +
			"EnsureDealContractsShareCompany")
	}
}

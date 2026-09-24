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
// The leak it closes is in the CONTRACT read, not this one. A contract with a
// deal is judged visible by that deal alone — deliberately, so that widening to
// the company does not hand out agreements attached to invisible deals — so a
// deal that moves to company B publishes company A's agreements to everyone who
// can see B's deal, and the agreement's own company is never consulted. Two
// "can you see it" checks cannot catch that; only asking whether the two name
// the same company can.
//
// It runs INSIDE the caller's transaction, and AFTER the deal row is locked,
// which is what makes it an answer rather than a guess: a contract being filed
// against this deal concurrently reads the deal's company under a share lock,
// so one of the two waits and sees what the other decided.
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

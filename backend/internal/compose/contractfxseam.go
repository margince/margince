// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The one place a contract's frozen conversion rate comes from.
//
// fx_rate belongs to deals, and the reading a contract freezes at activation is
// the same one a deal freezes at close: the latest rate on or before the day,
// against the installation's own base, with the same-currency shortcut and the
// same refusal when no rate exists. Contracts takes that as a seam rather than
// taking the module, so there is one spelling of the rule and no second one to
// drift from it.

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/modules/contracts"
	"github.com/margince/margince/backend/internal/modules/deals"
)

// ContractFreezeRate is the resolver every contract store is wired with.
func ContractFreezeRate(pool *pgxpool.Pool) contracts.FreezeRateFunc {
	store := deals.NewStore(InstallationDB(pool), DealsInstallation())
	return func(ctx context.Context, tx pgx.Tx, currency string, asOf time.Time) (string, time.Time, error) {
		return store.FreezeRateAt(ctx, tx, currency, asOf)
	}
}

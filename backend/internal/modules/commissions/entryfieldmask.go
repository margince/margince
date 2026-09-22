// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package commissions

// The ledger's field masks, applied where a row leaves the store for the wire.
//
// The tier an entry was accrued on is the partner's margin tier, frozen: a seat
// withheld the tier on the partner record reads it here unless this pass runs.
// The reach is declared in auth's group closure, not between the two modules —
// each withholds its own field and neither knows the other exists.

import (
	"context"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// commissionEntryWithholds are the fields a mask reaches on a ledger entry, and
// how each is withheld. margin_tier_at_accrual is deliberately not a pair an
// administrator can configure: it is withheld as a consequence of the partner's
// mask, and a second place to configure one fact is how an operator comes to
// believe the tier is hidden while one of the two says otherwise.
var commissionEntryWithholds = map[string]func(*crmcontracts.CommissionEntry){
	"margin_tier_at_accrual": func(e *crmcontracts.CommissionEntry) { e.MarginTierAtAccrual = nil },
}

// maskEntries withholds, per row, what this reader's role does not read.
// `commission` carries no owner of its own — its scope is inherited from the
// deal — so no mask on it can be conditioned on write authority and the pass
// costs no statement.
//
// It runs at the store's EXIT and never inside entryUnder, which is the read a
// void copies its reversal from: a masked row read there would insert a
// reversal with no tier, turning a withheld value into a wrong one.
func maskEntries(ctx context.Context, tx pgx.Tx, page []crmcontracts.CommissionEntry) error {
	return auth.ApplyFieldMasks(ctx, tx, commissionObject, page,
		func(e crmcontracts.CommissionEntry) ids.UUID { return ids.UUID(e.Id) },
		commissionEntryWithholds,
		func(e *crmcontracts.CommissionEntry, names []string) { e.MaskedFields = &names },
		nil, nil)
}

// maskedEntry is maskEntries over the ONE entry a single read answers with — or
// the echo of a write, which is a read too.
func maskedEntry(ctx context.Context, tx pgx.Tx, entry crmcontracts.CommissionEntry) (crmcontracts.CommissionEntry, error) {
	page := []crmcontracts.CommissionEntry{entry}
	if err := maskEntries(ctx, tx, page); err != nil {
		return crmcontracts.CommissionEntry{}, err
	}
	return page[0], nil
}

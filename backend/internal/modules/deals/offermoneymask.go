// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package deals

import (
	"context"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// withholdOfferMoney applies the deal's amount mask to offers on their way
// out. An offer's totals are the deal's amount by another name, and its lines
// rebuild it. So a reader masked from the amount gets the header only, with no
// totals, lines, rate or PDF. The legal-review pack follows the same rule.
func withholdOfferMoney(ctx context.Context, tx pgx.Tx, offers []crmcontracts.Offer) error {
	readable := map[ids.UUID]bool{}
	for i := range offers {
		deal := ids.UUID(offers[i].DealId)
		ok, seen := readable[deal]
		if !seen {
			var err error
			if ok, err = dealAmountReadable(ctx, tx, ids.From[ids.DealKind](deal)); err != nil {
				return err
			}
			readable[deal] = ok
		}
		if !ok {
			withholdMoneyOf(&offers[i])
		}
	}
	return nil
}

func withholdMoneyOf(o *crmcontracts.Offer) {
	o.NetMinor, o.TaxMinor, o.GrossMinor, o.ArrMinor, o.NetTcvMinor = nil, nil, nil, nil, nil
	o.FxRateToBase, o.FxRateDate, o.PdfAssetRef, o.DiffFromPrevious = nil, nil, nil, nil
	o.LineItems = &[]crmcontracts.OfferLineItem{}
	o.MoneyWithheld = new(true)
}

// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package salesprogress

import (
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
)

func TestSaleTypeReadsWhatThePricedLinesSell(t *testing.T) {
	amount := int64(60_000)
	for name, c := range map[string]struct {
		amount *int64
		lines  lineKinds
		want   crmcontracts.SalesProgressDealSaleType
	}{
		"all product":              {&amount, lineKinds{product: 60_000}, crmcontracts.SalesProgressDealSaleTypeProduct},
		"all service":              {&amount, lineKinds{service: 60_000}, crmcontracts.SalesProgressDealSaleTypeService},
		"both":                     {&amount, lineKinds{product: 30_000, service: 30_000}, crmcontracts.SalesProgressDealSaleTypeProductAndService},
		"an unpriced remainder":    {&amount, lineKinds{product: 40_000}, crmcontracts.SalesProgressDealSaleTypePartlyClassified},
		"a line of another kind":   {&amount, lineKinds{service: 50_000, other: 10_000}, crmcontracts.SalesProgressDealSaleTypePartlyClassified},
		"no lines":                 {&amount, lineKinds{}, crmcontracts.SalesProgressDealSaleTypeUnclassified},
		"only other kinds":         {&amount, lineKinds{other: 60_000}, crmcontracts.SalesProgressDealSaleTypeUnclassified},
		"lines over the deal":      {&amount, lineKinds{product: 50_000, service: 20_000}, crmcontracts.SalesProgressDealSaleTypeUnclassified},
		"a line in another money":  {&amount, lineKinds{product: 60_000, foreign: true}, crmcontracts.SalesProgressDealSaleTypeUnclassified},
		"a deal without an amount": {nil, lineKinds{product: 60_000}, crmcontracts.SalesProgressDealSaleTypeUnclassified},
	} {
		if got := saleType(c.amount, c.lines); got != c.want {
			t.Errorf("%s: saleType = %q, want %q", name, got, c.want)
		}
	}
}

// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package deals

import "testing"

func TestAFigureIsJudgedAsTheCallerWroteIt(t *testing.T) {
	t.Parallel()

	for name, tc := range map[string]struct {
		lim     figureLimit
		sent    float64
		refused bool
	}{
		"quantity with 3 places":        {quantityLimit, 1.234, false},
		"quantity with 4 places":        {quantityLimit, 1.2345, true},
		"quantity too small to keep":    {quantityLimit, 0.0004, true},
		"zero quantity":                 {quantityLimit, 0, true},
		"negative quantity":             {quantityLimit, -1, true},
		"quantity past its column":      {quantityLimit, 1e11, true},
		"discount at the top":           {discountLimit, 100, false},
		"discount past the top":         {discountLimit, 100.01, true},
		"discount with 3 places":        {discountLimit, 12.345, true},
		"discount of zero":              {discountLimit, 0, false},
		"negative tax":                  {taxLimit, -0.5, true},
		"tax with 2 places":             {taxLimit, 7.25, false},
		"tax past its column":           {taxLimit, 1000, true},
		"default tax rate is also held": {defaultTaxLimit, -1, true},
	} {
		err := checkFigure(tc.lim, wireDecimal(tc.sent))
		if (err != nil) != tc.refused {
			t.Errorf("%s: checkFigure(%v) = %v, want refused=%v", name, tc.sent, err, tc.refused)
		}
	}
}

func TestAMinorAmountMustFitWhatAJSONNumberHoldsExactly(t *testing.T) {
	t.Parallel()

	for amount, refused := range map[int64]bool{0: false, 100: false, maxPriceMinor: false, maxPriceMinor + 1: true, -1: true} {
		if err := checkPrice("unit_price_minor", amount); (err != nil) != refused {
			t.Errorf("checkPrice(%d) = %v, want refused=%v", amount, err, refused)
		}
	}
}

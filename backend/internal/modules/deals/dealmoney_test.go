// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package deals

import (
	"errors"
	"testing"
)

type fieldFaulter interface {
	FieldFault() (field, code, message string)
}

func TestADealAmountBelowZeroIsRefusedAtTheAmountField(t *testing.T) {
	eur := "EUR"
	for name, amount := range map[string]int64{"one below": -1, "far below": -500000} {
		t.Run(name, func(t *testing.T) {
			err := moneyPairError(&amount, nil, &eur)
			var fault fieldFaulter
			if !errors.As(err, &fault) {
				t.Fatalf("moneyPairError(%d) = %v, want a field fault", amount, err)
			}
			field, code, _ := fault.FieldFault()
			if field != amountField || code != "amount_negative" {
				t.Fatalf("fault = %s/%s, want %s/amount_negative", field, code, amountField)
			}
		})
	}
}

func TestADealAmountOfZeroOrMoreIsLegal(t *testing.T) {
	eur := "EUR"
	for _, amount := range []int64{0, 1, 99999} {
		if err := moneyPairError(&amount, nil, &eur); err != nil {
			t.Fatalf("moneyPairError(%d) = %v, want nil", amount, err)
		}
	}
}

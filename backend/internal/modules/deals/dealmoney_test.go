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
	for name, amount := range map[string]int64{"one below": -1, "far below": -500000} {
		t.Run(name, func(t *testing.T) {
			err := amountSetError(&amount)
			var fault fieldFaulter
			if !errors.As(err, &fault) {
				t.Fatalf("amountSetError(%d) = %v, want a field fault", amount, err)
			}
			field, code, _ := fault.FieldFault()
			if field != amountField || code != "amount_negative" {
				t.Fatalf("fault = %s/%s, want %s/amount_negative", field, code, amountField)
			}
		})
	}
}

func TestADealAmountOfZeroOrMoreIsLegal(t *testing.T) {
	for _, amount := range []int64{0, 1, 99999} {
		if err := amountSetError(&amount); err != nil {
			t.Fatalf("amountSetError(%d) = %v, want nil", amount, err)
		}
	}
	if err := amountSetError(nil); err != nil {
		t.Fatalf("amountSetError(nil) = %v, a cleared amount sets nothing below zero", err)
	}
}

// The pair rule answers on the resulting row, so it must not carry the sign
// rule: an older deal that already holds a negative amount stays editable.
func TestThePairRuleDoesNotJudgeTheSignOfAnAmountAlreadyOnTheRow(t *testing.T) {
	eur, amount := "EUR", int64(-5)
	if err := moneyPairError(&amount, nil, &eur); err != nil {
		t.Fatalf("moneyPairError(-5, EUR) = %v, want nil", err)
	}
}
